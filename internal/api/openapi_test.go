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
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
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
			name: "paginated concrete variant",
			data: `
package api
type PaginatedResponse[T any] struct {
	Items         []T    ` + "`" + `json:"items"` + "`" + `
	NextPageToken string ` + "`" + `json:"nextPageToken,omitempty"` + "`" + `
}
type Holder struct {
	List PaginatedResponse[Symbol] ` + "`" + `json:"list"` + "`" + `
}
type Symbol struct {
	Name string ` + "`" + `json:"name"` + "`" + `
}
`,
			want: `{
  "Holder": {
    "properties": {
      "list": {
        "$ref": "#/components/schemas/PaginatedResponse_Symbol"
      }
    },
    "type": "object"
  },
  "PaginatedResponse_Symbol": {
    "properties": {
      "items": {
        "items": {
          "$ref": "#/components/schemas/Symbol"
        },
        "type": "array"
      },
      "nextPageToken": {
        "type": "string"
      }
    },
    "type": "object"
  },
  "Symbol": {
    "properties": {
      "name": {
        "type": "string"
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
			want: `{
  "Package": {
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
  },
  "PackageInfo": {
    "properties": {
      "path": {
        "type": "string"
      },
      "synopsis": {
        "type": "string"
      }
    },
    "type": "object"
  }
}`,
		},
		{
			name: "instantiated generic",
			data: `
package api
type PackageImportedBy struct {
	ImportedBy PaginatedResponse[string] ` + "`" + `json:"importedBy"` + "`" + `
}
type PaginatedResponse[T any] struct {
	Items         []T    ` + "`" + `json:"items"` + "`" + `
	NextPageToken string ` + "`" + `json:"nextPageToken,omitempty"` + "`" + `
}
`,
			want: `{
  "PackageImportedBy": {
    "properties": {
      "importedBy": {
        "$ref": "#/components/schemas/PaginatedResponse_string"
      }
    },
    "type": "object"
  },
  "PaginatedResponse_string": {
    "properties": {
      "items": {
        "items": {
          "type": "string"
        },
        "type": "array"
      },
      "nextPageToken": {
        "type": "string"
      }
    },
    "type": "object"
  }
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateSchemas([]byte(tt.data), nil)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if got := string(data); got != tt.want {
				t.Errorf("generateSchemas() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestCollectTags(t *testing.T) {
	got := collectTags([]*RouteInfo{
		{Route: "/a", Tags: []string{"packages"}},
		{Route: "/b", Tags: []string{"module", "packages"}},
	})
	want := []openAPITag{
		{Name: "module"},
		{Name: "packages"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("collectTags() = %v, want %v", got, want)
	}
}

func TestValidateRefs(t *testing.T) {
	schemas := map[string]any{"Known": map[string]any{}}

	t.Run("resolved", func(t *testing.T) {
		doc := map[string]any{
			"a": []any{map[string]any{"$ref": "#/components/schemas/Known"}},
		}
		if err := validateRefs(doc, schemas); err != nil {
			t.Errorf("validateRefs() = %v, want nil", err)
		}
	})

	t.Run("dangling", func(t *testing.T) {
		doc := map[string]any{
			"a": []any{map[string]any{"$ref": "#/components/schemas/Missing"}},
		}
		if err := validateRefs(doc, schemas); err == nil {
			t.Error("validateRefs() = nil, want error for dangling reference")
		}
	})
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
				t.Errorf("API spec changed but version was not bumped from %s. Please update apiVersion in openapi_test.go or run with -update to bypass.", oldVersion)
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
	Tags       []openAPITag      `json:"tags"`
	Paths      map[string]any    `json:"paths"`
	Components openAPIComponents `json:"components"`
}

type openAPITag struct {
	Name string `json:"name"`
}

type openAPIInfo struct {
	Title       string         `json:"title"`
	Version     string         `json:"version"`
	Description string         `json:"description"`
	Contact     openAPIContact `json:"contact"`
}

type openAPIContact struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Email string `json:"email"`
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

	tags := collectTags(routes)

	spec := openAPISpec{
		OpenAPI: openAPISpecVersion,
		Info: openAPIInfo{
			Title:       "Go Pkgsite API",
			Version:     apiVersion,
			Description: "API for accessing information about Go packages and modules on pkg.go.dev.",
			Contact: openAPIContact{
				Name:  "The Go team at Google",
				URL:   "https://go.dev/s/discovery-feedback",
				Email: "golang-dev@googlegroups.com",
			},
		},
		Servers: []openAPIServer{
			{URL: "https://pkg.go.dev" + apiPathPrefix},
		},
		Tags:  tags,
		Paths: make(map[string]any),
	}

	for _, r := range routes {
		path := r.Route
		path = strings.TrimPrefix(path, apiPathPrefix)
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		operation := map[string]any{
			"summary":     r.Summary,
			"description": r.Desc,
			"operationId": generateOperationID(path),
			"tags":        r.Tags,
		}

		params := []map[string]any{}
		for _, p := range r.PathParams {
			params = append(params, map[string]any{
				"name":        p.Name,
				"in":          "path",
				"required":    true,
				"description": p.Doc,
				"schema": map[string]any{
					"type": "string",
				},
			})
		}

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
		if len(params) > 0 {
			operation["parameters"] = params
		}

		responses := map[string]any{
			"200": map[string]any{
				"description": "Successful response",
			},
			"default": map[string]any{
				"description": "Error response",
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{
							"$ref": "#/components/schemas/Error",
						},
					},
				},
			},
		}

		if r.ResponsePaginatedType != "" {
			responses["200"].(map[string]any)["content"] = map[string]any{
				"application/json": map[string]any{
					"schema": map[string]any{
						"$ref": "#/components/schemas/" + paginatedSchemaName(r.ResponsePaginatedType),
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

	var paginatedElems []string
	for _, r := range routes {
		if r.ResponsePaginatedType != "" {
			paginatedElems = append(paginatedElems, r.ResponsePaginatedType)
		}
	}

	schemas, err := generateSchemas(typesGo, paginatedElems)
	if err != nil {
		return "", err
	}
	spec.Components.Schemas = schemas

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", err
	}

	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	if err := validateRefs(doc, schemas); err != nil {
		return "", err
	}

	return string(data), nil
}

// validateRefs walks the decoded spec and returns an error for any
// "#/components/schemas/..." reference that has no matching schema, so that a
// dangling $ref (e.g. a paginated element type or response type with no struct)
// fails generation instead of producing an invalid spec.
func validateRefs(v any, schemas map[string]any) error {
	switch v := v.(type) {
	case map[string]any:
		for key, val := range v {
			if key == "$ref" {
				ref, ok := val.(string)
				if !ok {
					continue
				}
				name, ok := strings.CutPrefix(ref, "#/components/schemas/")
				if !ok {
					continue
				}
				if _, ok := schemas[name]; !ok {
					return fmt.Errorf("unresolved schema reference %q", ref)
				}
				continue
			}
			if err := validateRefs(val, schemas); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := validateRefs(item, schemas); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateSchemas(data []byte, paginatedElems []string) (map[string]any, error) {
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
		// PaginatedResponse is generic; concrete variants are emitted by
		// addPaginatedSchemas, so skip the generic base here.
		if name == "PaginatedResponse" {
			continue
		}
		properties := make(map[string]any)
		collectProperties(structType, structs, properties)
		schemas[name] = map[string]any{
			"type":       "object",
			"properties": properties,
		}
	}

	addPaginatedSchemas(schemas, structs, paginatedElems)

	return schemas, nil
}

// addPaginatedSchemas adds a concrete schema for each PaginatedResponse[T]
// instantiation, so that "items" references the actual element type instead of
// the generic object. Element types come both from struct fields and from the
// paginated response types declared by routes.
func addPaginatedSchemas(schemas map[string]any, structs map[string]*ast.StructType, paginatedElems []string) {
	base, ok := structs["PaginatedResponse"]
	if !ok {
		return
	}

	elems := map[string]bool{}
	for _, elem := range paginatedElems {
		elems[elem] = true
	}
	for _, st := range structs {
		for _, field := range st.Fields.List {
			if elem, ok := paginatedElem(typeExprToString(field.Type)); ok {
				elems[elem] = true
			}
		}
	}

	for elem := range elems {
		properties := make(map[string]any)
		collectProperties(base, structs, properties)
		properties["items"] = map[string]any{
			"type":  "array",
			"items": elemSchema(elem),
		}
		schemas[paginatedSchemaName(elem)] = map[string]any{
			"type":       "object",
			"properties": properties,
		}
	}
}

// paginatedElem reports whether t is a PaginatedResponse[E] type and returns E.
func paginatedElem(t string) (string, bool) {
	rest, ok := strings.CutPrefix(t, "PaginatedResponse[")
	if !ok || !strings.HasSuffix(rest, "]") {
		return "", false
	}
	return strings.TrimSuffix(rest, "]"), true
}

// paginatedSchemaName returns the component schema name for PaginatedResponse[elem].
func paginatedSchemaName(elem string) string {
	return "PaginatedResponse_" + elem
}

// elemSchema returns the OpenAPI schema for a single element of an array or
// paginated response with the given element type.
func elemSchema(elem string) map[string]any {
	switch elem {
	case "string", "bool", "int":
		return map[string]any{"type": mapType(elem)}
	case "T":
		return map[string]any{"type": "object"}
	default:
		return map[string]any{"$ref": "#/components/schemas/" + elem}
	}
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

		if !field.Names[0].IsExported() {
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
			return map[string]any{
				"type":  "array",
				"items": elemSchema(t[2:]),
			}
		} else if strings.HasPrefix(t, "*") {
			elem := t[1:]
			return map[string]any{"$ref": "#/components/schemas/" + elem}
		} else if elem, ok := paginatedElem(t); ok {
			return map[string]any{"$ref": "#/components/schemas/" + paginatedSchemaName(elem)}
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

// collectTags returns the global tags definition for all tags used by routes,
// sorted by name. Descriptions are left empty.
func collectTags(routes []*RouteInfo) []openAPITag {
	tags := make(map[string]openAPITag)
	for _, r := range routes {
		for _, name := range r.Tags {
			tags[name] = openAPITag{Name: name}
		}
	}
	return slices.SortedFunc(maps.Values(tags), func(a, b openAPITag) int {
		return strings.Compare(a.Name, b.Name)
	})
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
