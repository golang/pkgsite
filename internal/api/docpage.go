// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/sync/errgroup"
)

// QueryParam contains information about a query parameter.
type QueryParam struct {
	Name string
	Type string
	Doc  string
}

// PathParam contains information about a path parameter.
type PathParam struct {
	Name string
	Doc  string
}

// Example contains an API request example (URL path) and its expected response.
type Example struct {
	Request  string
	Response string
}

// RouteInfo contains documentation information for an API route.
type RouteInfo struct {
	Route                 string
	Tags                  []string
	Summary               string
	Desc                  string
	Params                string
	Response              string
	ResponsePaginatedType string
	LinkPaginatedType     bool
	PathParams            []PathParam
	QueryParams           []QueryParam
	Examples              []*Example
}

// parseParamsFile parses a Go source file containing parameter structs
// and returns a map from struct name to its query parameters.
func parseParamsFile(data []byte) (map[string][]QueryParam, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", data, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// Do this in two phases, so we can find embedded structs even if
	// they're later in the file.

	// Phase 1: collect params structs.
	structs := make(map[string]*ast.StructType)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || !strings.HasSuffix(typeSpec.Name.Name, "Params") {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			structs[typeSpec.Name.Name] = structType
		}
	}

	// Phase 2: build query params.
	paramsMap := make(map[string][]QueryParam)

	// processStruct builds the query params for the given struct
	// and puts them in paramsMap.
	var processStruct func(string, *ast.StructType)
	processStruct = func(name string, st *ast.StructType) {
		var params []QueryParam
		for _, field := range st.Fields.List {
			// field.Names is nil for embedded structs.
			if field.Names == nil {
				typeName := field.Type.(*ast.Ident).Name

				if paramsMap[typeName] == nil {
					est := structs[typeName]
					if est == nil {
						panic(fmt.Sprintf("unknown embedded type %q", typeName))
					}
					// This recursion must bottom out because embeddings
					// can't form a cycle.
					processStruct(typeName, est)
				}
				params = append(params, paramsMap[typeName]...)
				continue
			}

			tag := ""
			if field.Tag != nil {
				tag = field.Tag.Value
			}
			formName := extractFormName(tag)
			if formName == "" {
				continue
			}

			doc := ""
			if field.Doc != nil {
				doc = strings.TrimSpace(field.Doc.Text())
			}

			params = append(params, QueryParam{
				Name: formName,
				Type: exprToString(field.Type),
				Doc:  doc,
			})

		}
		paramsMap[name] = params
	}

	for name, structType := range structs {
		processStruct(name, structType)
	}
	return paramsMap, nil
}

// extractFormName extracts the query parameter name from a struct field's form tag.
func extractFormName(tag string) string {
	if tag == "" {
		return ""
	}
	tag = strings.Trim(tag, "`")
	structTag := reflect.StructTag(tag)
	formVal := structTag.Get("form")
	name, _, _ := strings.Cut(formVal, ",")
	return name
}

func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.ArrayType:
		return "[]" + exprToString(e.Elt)
	default:
		return fmt.Sprintf("%T", expr)
	}
}

//go:embed params.go
var paramsGo []byte

//go:embed api.go
var apiGo []byte

var (
	routesMu sync.Mutex
	routes   []*RouteInfo
	routeErr error
)

// RouteInfos returns the documentation information for all routes,
// and executes examples against the given baseURL if they haven't been executed yet.
func RouteInfos(ctx context.Context, baseURL string) ([]*RouteInfo, error) {
	routesMu.Lock()
	defer routesMu.Unlock()
	if routes == nil && routeErr == nil {
		routes, routeErr = calculateRoutes(ctx, baseURL)
	}
	return routes, routeErr
}

func calculateRoutes(ctx context.Context, baseURL string) ([]*RouteInfo, error) {
	paramsMap, err := parseParamsFile(paramsGo)
	if err != nil {
		return nil, err
	}
	routes, err := readRouteInfo(apiGo, paramsMap)
	if err != nil {
		return nil, err
	}
	if err := executeExamples(ctx, baseURL, routes); err != nil {
		return nil, err
	}
	return routes, nil
}

var apiRE = regexp.MustCompile(`//\s*api:(\S+)\s+(.*)`)

// routePlaceholderRE matches path placeholders in a route, e.g. {path} in /v1beta/module/{path}.
var routePlaceholderRE = regexp.MustCompile(`\{([^}]+)\}`)

// routeTag returns the tag for a route: the path element after the first.
// For example, the tag for "/v1beta/package/{path}" is "package".
// It returns "default" when the route has no such element.
func routeTag(route string) string {
	parts := strings.Split(strings.Trim(route, "/"), "/")
	if len(parts) < 2 {
		return "default"
	}
	return parts[1]
}

// readRouteInfo reads the provided Go source data and returns documentation information for all routes.
func readRouteInfo(data []byte, paramsMap map[string][]QueryParam) ([]*RouteInfo, error) {
	var routes []*RouteInfo
	var current *RouteInfo

	add := func(r *RouteInfo) error {
		if r == nil {
			return nil
		}
		if r.Route == "" {
			return errors.New("missing api:route")
		}
		if slices.ContainsFunc(routes, func(ex *RouteInfo) bool { return ex.Route == r.Route }) {
			return fmt.Errorf("duplicate api:route %q", r.Route)
		}
		if r.Desc == "" {
			return fmt.Errorf("missing api:desc field in route %q", r.Route)
		}
		r.Tags = []string{routeTag(r.Route)}
		r.Summary, _, _ = strings.Cut(r.Desc, ".")
		if r.Params == "" {
			return fmt.Errorf("missing api:params field in route %q", r.Route)
		}
		if r.Response == "" {
			return fmt.Errorf("missing api:response field in route %q", r.Route)
		}

		placeholders := map[string]bool{}
		for _, m := range routePlaceholderRE.FindAllStringSubmatch(r.Route, -1) {
			placeholders[m[1]] = true
		}
		declared := map[string]bool{}
		for _, p := range r.PathParams {
			if !placeholders[p.Name] {
				return fmt.Errorf("api:pathparam %q is not a placeholder in route %q", p.Name, r.Route)
			}
			declared[p.Name] = true
		}
		for name := range placeholders {
			if !declared[name] {
				return fmt.Errorf("route %q has placeholder %q with no api:pathparam", r.Route, name)
			}
		}

		routes = append(routes, r)
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m := apiRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, val := m[1], strings.TrimSpace(m[2])
		if val == "" {
			return nil, fmt.Errorf("missing value for key %q", key)
		}

		switch key {
		case "route":
			if err := add(current); err != nil {
				return nil, err
			}
			current = &RouteInfo{Route: val}
		case "desc":
			if current == nil {
				return nil, fmt.Errorf("saw api:desc before api:route")
			}
			if current.Desc == "" {
				current.Desc = val
			} else {
				current.Desc += "\n" + val
			}
		case "pathparam":
			if current == nil {
				return nil, fmt.Errorf("saw api:pathparam before api:route")
			}
			name, doc, _ := strings.Cut(val, " ")
			doc = strings.TrimSpace(doc)
			if doc == "" {
				return nil, fmt.Errorf("missing description for api:pathparam %q in route %q", name, current.Route)
			}
			current.PathParams = append(current.PathParams, PathParam{Name: name, Doc: doc})
		case "params":
			if current == nil {
				return nil, fmt.Errorf("saw api:params before api:route")
			}
			if current.Params != "" {
				return nil, fmt.Errorf("duplicate api:params in route %q", current.Route)
			}
			current.Params = val
			if qps, ok := paramsMap[val]; ok {
				current.QueryParams = qps
			}
		case "response":
			if current == nil {
				return nil, fmt.Errorf("saw api:response before api:route")
			}
			if current.Response != "" {
				return nil, fmt.Errorf("duplicate api:response in route %q", current.Route)
			}
			current.Response = val
			if before, after, _ := strings.Cut(val, "["); before == "PaginatedResponse" && strings.HasSuffix(after, "]") {
				current.ResponsePaginatedType = after[:len(after)-1]
				if len(current.ResponsePaginatedType) > 0 {
					current.LinkPaginatedType = !unicode.IsLower(rune(current.ResponsePaginatedType[0]))
				}
			}
		case "example":
			if current == nil {
				return nil, fmt.Errorf("saw api:example before api:route")
			}
			current.Examples = append(current.Examples, &Example{Request: val})
		default:
			route := "(unknown route)"
			if current != nil {
				route = current.Route
			}
			return nil, fmt.Errorf("unknown api key %q in route %s", key, route)
		}
	}
	if err := add(current); err != nil {
		return nil, err
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("no routes found")
	}
	return routes, nil
}

// executeExamples executes actual HTTP requests against the given baseURL for all examples
// found in the provided routes, and populates their Response fields with the resulting bodies.
func executeExamples(ctx context.Context, baseURL string, routes []*RouteInfo) error {
	client := &http.Client{Timeout: 5 * time.Second}
	base, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("parsing base URL %q: %w", baseURL, err)
	}

	// Make requests for example responses concurrently.
	g, ctx := errgroup.WithContext(ctx)
	for _, r := range routes {
		for _, ex := range r.Examples {
			rel, err := url.Parse(ex.Request)
			if err != nil {
				return fmt.Errorf("parsing example request %q: %w", ex.Request, err)
			}
			g.Go(func() error {
				urlStr := base.ResolveReference(rel).String()
				req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
				if err != nil {
					return fmt.Errorf("creating request for %q: %w", urlStr, err)
				}
				resp, err := client.Do(req)
				if err != nil {
					ex.Response = fmt.Sprintf("getting response: %v", err)
					return nil
				}

				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					ex.Response = fmt.Sprintf("reading response: %v", err)
					return nil
				}
				var formatted bytes.Buffer
				if err := json.Indent(&formatted, body, "", "  "); err != nil {
					ex.Response = fmt.Sprintf("indenting response: %v", err)
				} else {
					ex.Response = formatted.String()
				}
				return nil
			})
		}
	}
	return g.Wait()
}
