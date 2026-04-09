// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bufio"
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// RouteInfo contains documentation information for an API route.
type RouteInfo struct {
	Route    string
	Desc     string
	Params   string
	Response string
}

//go:embed api.go
var apiGo []byte

var RouteInfos = sync.OnceValues(func() ([]*RouteInfo, error) {
	return readRouteInfo(apiGo)
})

var apiRE = regexp.MustCompile(`//\s*api:(\S+)\s+(.*)`)

// readRouteInfo reads the provided Go source data and returns documentation information for all routes.
func readRouteInfo(data []byte) ([]*RouteInfo, error) {
	var routes []*RouteInfo
	var current *RouteInfo

	add := func(r *RouteInfo) error {
		if r == nil {
			return nil
		}
		if r.Route == "" {
			return errors.New("missing api:route")
		}
		if r.Desc == "" {
			return fmt.Errorf("missing api:desc field in route %q", r.Route)
		}
		if r.Params == "" {
			return fmt.Errorf("missing api:params field in route %q", r.Route)
		}
		if r.Response == "" {
			return fmt.Errorf("missing api:params field in route %q", r.Route)
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
			if current.Desc != "" {
				return nil, fmt.Errorf("duplicate api:desc in route %q", current.Route)
			}
			current.Desc = val
		case "params":
			if current == nil {
				return nil, fmt.Errorf("saw api:params before api:route")
			}
			if current.Params != "" {
				return nil, fmt.Errorf("duplicate api:params in route %q", current.Route)
			}
			current.Params = val
		case "response":
			if current == nil {
				return nil, fmt.Errorf("saw api:response before api:route")
			}
			if current.Response != "" {
				return nil, fmt.Errorf("duplicate api:response in route %q", current.Route)
			}
			current.Response = val
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
