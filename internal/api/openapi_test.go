// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/pkgsite/internal/testing/testhelper"
)

func TestGenerateSchemas(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "basic types",
			data: `
package api
type Basic struct {
	Field1 string ` + "`" + `json:"field1"` + "`" + `
	Field2 bool   ` + "`" + `json:"field2"` + "`" + `
}
`,
			want: `{
  "Basic": {
    "properties": {
      "field1": {
        "type": "string"
      },
      "field2": {
        "type": "boolean"
      }
    },
    "type": "object"
  }
}`,
		},
		{
			name: "pointers and arrays",
			data: `
package api
type Complex struct {
	PtrField *Readme ` + "`" + `json:"ptrField"` + "`" + `
	ArrField []License ` + "`" + `json:"arrField"` + "`" + `
}
`,
			want: `{
  "Complex": {
    "properties": {
      "arrField": {
        "items": {
          "$ref": "#/components/schemas/License"
        },
        "type": "array"
      },
      "ptrField": {
        "$ref": "#/components/schemas/Readme"
      }
    },
    "type": "object"
  }
}`,
		},
		{
			name: "generics elision",
			data: `
package api
type PaginatedResponse[T any] struct {
	Items []T ` + "`" + `json:"items"` + "`" + `
}
`,
			want: `{
  "PaginatedResponse": {
    "properties": {
      "items": {
        "items": {
          "type": "object"
        },
        "type": "array"
      }
    },
    "type": "object"
  }
}`,
		},
		{
			name: "embedded struct",
			data: `
package api
type Package struct {
	Version string ` + "`" + `json:"version"` + "`" + `
	PackageInfo
}
type PackageInfo struct {
	Path     string ` + "`" + `json:"path"` + "`" + `
	Synopsis string ` + "`" + `json:"synopsis"` + "`" + `
}
`,
			want: `"Package": {
    "properties": {
      "path": {
        "type": "string"
      },
      "synopsis": {
        "type": "string"
      },
      "version": {
        "type": "string"
      }
    },
    "type": "object"
  }`,
		},
		{
			name: "instantiated generic",
			data: `
package api
type PackageImportedBy struct {
	ImportedBy PaginatedResponse[string] ` + "`" + `json:"importedBy"` + "`" + `
}
`,
			want: `{
  "PackageImportedBy": {
    "properties": {
      "importedBy": {
        "$ref": "#/components/schemas/PaginatedResponse"
      }
    },
    "type": "object"
  }
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateSchemas([]byte(tt.data))
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			gotStr := string(data)
			if !strings.Contains(gotStr, tt.want) {
				t.Errorf("generateSchemas output does not contain expected schema.\nWant:\n%s\nGot:\n%s", tt.want, gotStr)
			}
		})
	}
}

var update = flag.Bool("update", false, "update goldens instead of checking against them")

func TestGenerateOpenAPI(t *testing.T) {
	got, err := GenerateOpenAPI()
	if err != nil {
		t.Fatal(err)
	}

	if !*update {
		oldBytes, err := os.ReadFile("openapi.yaml")
		if err == nil {
			oldStr := string(oldBytes)
			oldVersion := extractVersion(oldStr)
			newVersion := extractVersion(got)

			normOld := normalizeSpec(oldStr)
			normNew := normalizeSpec(got)

			if normOld != normNew && oldVersion == newVersion {
				t.Errorf("API spec changed but version was not bumped from %s. Please update apiVersion in openapi.go or run with -update to bypass.", oldVersion)
			}
		}
	}

	testhelper.CompareWithGolden(t, got, "../openapi.yaml", *update)
}

func extractVersion(s string) string {
	re := regexp.MustCompile(`(?m)^  version: (.*)$`)
	matches := re.FindStringSubmatch(s)
	if len(matches) > 1 {
		return strings.Trim(matches[1], `"`)
	}
	return ""
}

func normalizeSpec(s string) string {
	re := regexp.MustCompile(`(?m)^  version: .*$`)
	return re.ReplaceAllString(s, "  version: __VERSION__")
}

//go:embed types.go
var typesGo []byte

type openAPISpec struct {
	OpenAPI    string            `json:"openapi"`
	Info       openAPIInfo       `json:"info"`
	Servers    []openAPIServer   `json:"servers"`
	Paths      map[string]any    `json:"paths"`
	Components openAPIComponents `json:"components"`
}

type openAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type openAPIServer struct {
	URL string `json:"url"`
}

type openAPIComponents struct {
	Schemas map[string]any `json:"schemas"`
}

// GenerateOpenAPI generates an OpenAPI 3.0 specification in JSON format
// (which is valid YAML) based on the routes returned by RouteInfos.
func GenerateOpenAPI() (string, error) {
	const (
		openAPISpecVersion = "3.0.3"
		apiVersion         = "v0.1.1"
		apiPathPrefix      = "/v1beta"
	)

	routes, err := RouteInfos(context.TODO(), "")
	if err != nil {
		return "", err
	}

	spec := openAPISpec{
		OpenAPI: openAPISpecVersion,
		Info: openAPIInfo{
			Title:       "Go Pkgsite API",
			Version:     apiVersion,
			Description: "API for accessing information about Go packages and modules on pkg.go.dev.",
		},
		Servers: []openAPIServer{
			{URL: "https://pkg.go.dev" + apiPathPrefix},
		},
		Paths: make(map[string]any),
	}

	for _, r := range routes {
		path := r.Route
		path = strings.TrimPrefix(path, apiPathPrefix)
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		operation := map[string]any{
			"summary":     r.Desc,
			"operationId": generateOperationID(path),
		}

		if len(r.QueryParams) > 0 {
			params := []map[string]any{}
			for _, p := range r.QueryParams {
				params = append(params, map[string]any{
					"name":        p.Name,
					"in":          "query",
					"description": p.Doc,
					"schema": map[string]any{
						"type": mapType(p.Type),
					},
				})
			}
			operation["parameters"] = params
		}

		responses := map[string]any{
			"200": map[string]any{
				"description": "Successful response",
			},
		}

		if r.ResponsePaginatedType != "" {
			responses["200"].(map[string]any)["content"] = map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/PaginatedResponse",
					},
				},
			}
		} else if r.Response != "" {
			responses["200"].(map[string]any)["content"] = map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/" + r.Response,
					},
				},
			}
		}

		operation["responses"] = responses
		spec.Paths[path] = map[string]any{
			"get": operation,
		}
	}

	schemas, err := generateSchemas(typesGo)
	if err != nil {
		return "", err
	}
	spec.Components.Schemas = schemas

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func generateSchemas(data []byte) (map[string]any, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", data, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	structs := make(map[string]*ast.StructType)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typeName := typeSpec.Name.Name
			if structType, ok := typeSpec.Type.(*ast.StructType); ok {
				structs[typeName] = structType
			}
		}
	}

	schemas := make(map[string]any)
	for name, structType := range structs {
		properties := make(map[string]any)
		collectProperties(structType, structs, properties)
		schemas[name] = map[string]any{
			"type":       "object",
			"properties": properties,
		}
	}

	return schemas, nil
}

// collectProperties adds the schema property for each field of st to properties,
// recursing into embedded structs so their fields are promoted to the parent.
func collectProperties(st *ast.StructType, structs map[string]*ast.StructType, properties map[string]any) {
	for _, field := range st.Fields.List {
		if field.Names == nil {
			if embedded, ok := structs[typeExprToString(field.Type)]; ok {
				collectProperties(embedded, structs, properties)
			}
			continue
		}

		fieldName := field.Names[0].Name
		tag := ""
		if field.Tag != nil {
			tag = field.Tag.Value
		}
		jsonName := extractJSONName(tag)
		if jsonName == "" {
			jsonName = fieldName
		}

		prop := mapFieldType(typeExprToString(field.Type))
		if field.Doc != nil {
			prop["description"] = strings.TrimSpace(field.Doc.Text())
		} else if field.Comment != nil {
			prop["description"] = strings.TrimSpace(field.Comment.Text())
		}
		properties[jsonName] = prop
	}
}

func mapFieldType(t string) map[string]any {
	switch t {
	case "string":
		return map[string]any{"type": "string"}
	case "time.Time":
		return map[string]any{"type": "string", "format": "date-time"}
	case "bool":
		return map[string]any{"type": "boolean"}
	case "int":
		return map[string]any{"type": "integer"}
	default:
		if strings.HasPrefix(t, "[]") {
			elem := t[2:]
			items := map[string]any{}
			switch elem {
			case "string", "bool", "int":
				items["type"] = mapType(elem)
			case "T":
				items["type"] = "object"
			default:
				items["$ref"] = "#/components/schemas/" + elem
			}
			return map[string]any{
				"type":  "array",
				"items": items,
			}
		} else if strings.HasPrefix(t, "*") {
			elem := t[1:]
			return map[string]any{"$ref": "#/components/schemas/" + elem}
		} else if strings.HasPrefix(t, "PaginatedResponse[") {
			return map[string]any{"$ref": "#/components/schemas/PaginatedResponse"}
		} else {
			return map[string]any{"$ref": "#/components/schemas/" + t}
		}
	}
}

func extractJSONName(tag string) string {
	if tag == "" {
		return ""
	}
	tag = strings.Trim(tag, "`")
	structTag := reflect.StructTag(tag)
	jsonVal := structTag.Get("json")
	name, _, _ := strings.Cut(jsonVal, ",")
	return name
}

func typeExprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.ArrayType:
		return "[]" + typeExprToString(e.Elt)
	case *ast.StarExpr:
		return "*" + typeExprToString(e.X)
	case *ast.IndexExpr:
		// Handle generic types like PaginatedResponse[SearchResult]
		return typeExprToString(e.X) + "[" + typeExprToString(e.Index) + "]"
	case *ast.SelectorExpr:
		return typeExprToString(e.X) + "." + e.Sel.Name
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func generateOperationID(path string) string {
	var sb strings.Builder
	sb.WriteString("get")
	for p := range strings.SplitSeq(path, "/") {
		if p == "" || strings.HasPrefix(p, "{") {
			continue
		}
		if len(p) > 0 {
			sb.WriteString(strings.ToUpper(p[:1]))
			sb.WriteString(p[1:])
		}
	}
	return sb.String()
}

func mapType(t string) string {
	switch t {
	case "bool":
		return "boolean"
	case "int":
		return "integer"
	default:
		return "string"
	}
}
