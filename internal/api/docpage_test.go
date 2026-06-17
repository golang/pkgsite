// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type DummyParams struct {
	Param1 string `form:"p1, doc for p1"`
	Param2 int    `form:"p2, doc for p2"`
}

type DummyListParams struct {
	Limit int `form:"limit, limit doc"`
}

type DummyComplexParams struct {
	DummyListParams
	Param3 bool `form:"p3"`
}

func TestReadRouteInfo(t *testing.T) {
	for _, test := range []struct {
		name      string
		data      string
		paramsMap map[string][]QueryParam
		want      []*RouteInfo
		wantErr   bool
	}{
		{
			name: "with query params",
			data: `
//api:route /v1beta/dummy
//api:desc Dummy route.
//api:params DummyParams
//api:response DummyResponse
`,
			paramsMap: map[string][]QueryParam{
				"DummyParams": {
					{Name: "p1", Type: "string", Doc: "doc for p1"},
					{Name: "p2", Type: "int", Doc: "doc for p2"},
				},
			},
			want: []*RouteInfo{
				{
					Route:    "/v1beta/dummy",
					Tags:     []string{"dummy"},
					Summary:  "Dummy route",
					Desc:     "Dummy route.",
					Params:   "DummyParams",
					Response: "DummyResponse",
					QueryParams: []QueryParam{
						{Name: "p1", Type: "string", Doc: "doc for p1"},
						{Name: "p2", Type: "int", Doc: "doc for p2"},
					},
				},
			},
		},
		{
			name: "with complex query params",
			data: `
//api:route /v1beta/dummy-complex
//api:desc Dummy complex route.
//api:params DummyComplexParams
//api:response DummyComplexResponse
`,
			paramsMap: map[string][]QueryParam{
				"DummyComplexParams": {
					{Name: "limit", Type: "int", Doc: "limit doc"},
					{Name: "p3", Type: "bool", Doc: ""},
				},
			},
			want: []*RouteInfo{
				{
					Route:    "/v1beta/dummy-complex",
					Tags:     []string{"dummy-complex"},
					Summary:  "Dummy complex route",
					Desc:     "Dummy complex route.",
					Params:   "DummyComplexParams",
					Response: "DummyComplexResponse",
					QueryParams: []QueryParam{
						{Name: "limit", Type: "int", Doc: "limit doc"},
						{Name: "p3", Type: "bool", Doc: ""},
					},
				},
			},
		},
		{
			name: "correct",
			data: `
//api:route /v1beta/package/{path}
//api:pathparam path Module or package path.
//api:desc Get package metadata.
//api:params path, version, module
//api:response Package
//api:route /v1beta/module/{path}
//api:pathparam path Module or package path.
//api:desc Get module metadata.
//api:params path, version
//api:response Module
`,
			want: []*RouteInfo{
				{
					Route:      "/v1beta/package/{path}",
					Tags:       []string{"package"},
					Summary:    "Get package metadata",
					Desc:       "Get package metadata.",
					Params:     "path, version, module",
					Response:   "Package",
					PathParams: []PathParam{{Name: "path", Doc: "Module or package path."}},
				},
				{
					Route:      "/v1beta/module/{path}",
					Tags:       []string{"module"},
					Summary:    "Get module metadata",
					Desc:       "Get module metadata.",
					Params:     "path, version",
					Response:   "Module",
					PathParams: []PathParam{{Name: "path", Doc: "Module or package path."}},
				},
			},
		},
		{
			name: "paginated",
			data: `
//api:route /v1beta/versions/{path}
//api:pathparam path Module or package path.
//api:desc All versions of the module at {path}.
//api:params filter, limit, token
//api:response PaginatedResponse[ModuleInfo]
`,
			want: []*RouteInfo{
				{
					Route:                 "/v1beta/versions/{path}",
					Tags:                  []string{"versions"},
					Summary:               "All versions of the module at {path}",
					Desc:                  "All versions of the module at {path}.",
					Params:                "filter, limit, token",
					Response:              "PaginatedResponse[ModuleInfo]",
					ResponsePaginatedType: "ModuleInfo",
					LinkPaginatedType:     true,
					PathParams:            []PathParam{{Name: "path", Doc: "Module or package path."}},
				},
			},
		},
		{
			name: "paginated lower",
			data: `
//api:route /v1beta/strings
//api:desc Some strings.
//api:params filter
//api:response PaginatedResponse[string]
`,
			want: []*RouteInfo{
				{
					Route:                 "/v1beta/strings",
					Tags:                  []string{"strings"},
					Summary:               "Some strings",
					Desc:                  "Some strings.",
					Params:                "filter",
					Response:              "PaginatedResponse[string]",
					ResponsePaginatedType: "string",
					LinkPaginatedType:     false,
				},
			},
		},
		{
			name: "multi-sentence description",
			data: `
//api:route /v1beta/vulns/{path}
//api:pathparam path Module or package path.
//api:desc Vulnerabilities of the module or package at {path}.
//api:desc Data comes from the Go vulnerability database.
//api:desc Only results that match the filter query parameter are returned.
//api:params filter
//api:response PaginatedResponse[Vulnerability]
`,
			want: []*RouteInfo{
				{
					Route:                 "/v1beta/vulns/{path}",
					Tags:                  []string{"vulns"},
					Summary:               "Vulnerabilities of the module or package at {path}",
					Desc:                  "Vulnerabilities of the module or package at {path}.\nData comes from the Go vulnerability database.\nOnly results that match the filter query parameter are returned.",
					Params:                "filter",
					Response:              "PaginatedResponse[Vulnerability]",
					ResponsePaginatedType: "Vulnerability",
					LinkPaginatedType:     true,
					PathParams:            []PathParam{{Name: "path", Doc: "Module or package path."}},
				},
			},
		},
		{
			name: "multiple sentences on first line",
			data: `
//api:route /v1beta/search
//api:desc Search results. Only matching results are returned.
//api:params filter
//api:response PaginatedResponse[SearchResult]
`,
			want: []*RouteInfo{
				{
					Route:                 "/v1beta/search",
					Tags:                  []string{"search"},
					Summary:               "Search results",
					Desc:                  "Search results. Only matching results are returned.",
					Params:                "filter",
					Response:              "PaginatedResponse[SearchResult]",
					ResponsePaginatedType: "SearchResult",
					LinkPaginatedType:     true,
				},
			},
		},
		{
			name: "missing field",
			data: `
//api:route /v1beta/package/{path}
//api:desc Get package metadata.
//api:pathparam path Module or package path.
//api:response Package
`,
			wantErr: true,
		},
		{
			name:    "no routes",
			data:    "package api\n\n// some other comment",
			wantErr: true,
		},
		{
			name: "empty value",
			data: `
//api:route /v1beta/package/{path}
//api:desc
//api:pathparam path Module or package path.
`,
			wantErr: true,
		},
		{
			name: "unknown key",
			data: `
//api:route /v1beta/package/{path}
//api:pathparam path Module or package path.
//api:unknown something
`,
			wantErr: true,
		},
		{
			name: "duplicate route",
			data: `
//api:route /v1beta/package/{path}
//api:pathparam path Module or package path.
//api:desc Get package metadata.
//api:params DummyParams
//api:response Package
//api:route /v1beta/package/{path}
//api:pathparam path Module or package path.
//api:desc Get package metadata.
//api:params DummyParams
//api:response Package
`,
			wantErr: true,
		},
		{
			name: "pathparam before route",
			data: `
//api:pathparam path Module or package path.
//api:route /v1beta/package/{path}
`,
			wantErr: true,
		},
		{
			name: "pathparam missing description",
			data: `
//api:route /v1beta/package/{path}
//api:pathparam path
//api:desc Get package metadata.
//api:params DummyParams
//api:response Package
`,
			wantErr: true,
		},
		{
			name: "placeholder without pathparam",
			data: `
//api:route /v1beta/package/{path}
//api:desc Get package metadata.
//api:params DummyParams
//api:response Package
`,
			wantErr: true,
		},
		{
			name: "pathparam name mismatch",
			data: `
//api:route /v1beta/package/{path}
//api:pathparam pth Module or package path.
//api:desc Get package metadata.
//api:params DummyParams
//api:response Package
`,
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := readRouteInfo([]byte(test.data), test.paramsMap)
			if (err != nil) != test.wantErr {
				t.Errorf("ReadRouteInfo() error = %v, wantErr %v", err, test.wantErr)
				return
			}
			if !test.wantErr {
				if diff := cmp.Diff(test.want, got); diff != "" {
					t.Errorf("ReadRouteInfo() mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestRouteInfos(t *testing.T) {
	origApiGo := apiGo
	defer func() { apiGo = origApiGo }()

	apiGo = []byte(`
//api:route /v1/dummy
//api:desc Dummy route.
//api:params DummyParams
//api:response DummyResponse
//api:example /v1/dummy
`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/dummy" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result": "dummy"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	routes, err := calculateRoutes(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("RouteInfos failed: %v", err)
	}

	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	if len(routes[0].Examples) != 1 {
		t.Fatalf("expected 1 example, got %d", len(routes[0].Examples))
	}

	wantResp := `{
  "result": "dummy"
}`
	if routes[0].Examples[0].Response != wantResp {
		t.Errorf("expected response %q, got %q", wantResp, routes[0].Examples[0].Response)
	}
}

func TestParseParamsAST(t *testing.T) {
	data := `
package api
type DummyParams struct {
	// doc for p1
	Param1 string ` + "`" + `form:"p1"` + "`" + `
	Param2 int    ` + "`" + `form:"p2"` + "`" + `
}
type DummyComplexParams struct {
	DummyListParams
	Param3 bool ` + "`" + `form:"p3"` + "`" + `
}
type DummyListParams struct {
	// limit doc
	Limit int ` + "`" + `form:"limit"` + "`" + `
}
`
	want := map[string][]QueryParam{
		"DummyParams": {
			{Name: "p1", Type: "string", Doc: "doc for p1"},
			{Name: "p2", Type: "int", Doc: ""},
		},
		"DummyListParams": {
			{Name: "limit", Type: "int", Doc: "limit doc"},
		},
		"DummyComplexParams": {
			{Name: "limit", Type: "int", Doc: "limit doc"},
			{Name: "p3", Type: "bool", Doc: ""},
		},
	}

	got, err := parseParamsFile([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("parseParams mismatch (-want +got):\n%s", diff)
	}
}

func TestExecuteExamples(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/search" && r.URL.Query().Get("q") == "Synopsis" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	routes := []*RouteInfo{
		{
			Route: "/v1/search",
			Examples: []*Example{
				{Request: "/v1/search?q=Synopsis"},
			},
		},
	}

	ctx := context.Background()
	if err := executeExamples(ctx, srv.URL, routes); err != nil {
		t.Fatalf("executeExamples: %v", err)
	}

	wantResp := `{
  "status": "ok"
}`
	if routes[0].Examples[0].Response != wantResp {
		t.Errorf("expected Response to be %q, got %q", wantResp, routes[0].Examples[0].Response)
	}
}
