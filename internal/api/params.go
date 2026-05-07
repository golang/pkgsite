// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
)

// ListParams are common pagination and filtering parameters.
type ListParams struct {
	// max number of items to return
	Limit int `form:"limit"`
	// where to resume listing
	Token string `form:"token"`
	// include only items matching filter
	Filter string `form:"filter"`
}

// PackageParams are query parameters for /v1beta/package/{path}.
type PackageParams struct {
	// module path
	Module string `form:"module"`
	// module version (latest if empty)
	Version string `form:"version"`
	// GOOS of documentation build context
	GOOS string `form:"goos"`
	// GOARCH of documentation build context
	GOARCH string `form:"goarch"`
	// Documentation format: text, html, md or markdown.
	// If omitted, documentation is not returned.
	Doc string `form:"doc"`
	// Whether to include examples with the returned documentation.
	Examples bool `form:"examples"`
	// Whether to include the packages that this one imports.
	Imports bool `form:"imports"`
	// Whether to include licenses in the result.
	Licenses bool `form:"licenses"`
}

// SymbolsParams are query parameters for /v1beta/symbols/{path}.
type SymbolsParams struct {
	// module path
	Module string `form:"module"`
	// module version (latest if empty)
	Version string `form:"version"`
	// GOOS of documentation build context
	GOOS string `form:"goos"`
	// GOARCH of documentation build context
	GOARCH string `form:"goarch"`
	ListParams
}

// ImportedByParams are query parameters for /v1beta/imported-by/{path}.
type ImportedByParams struct {
	// module path
	Module string `form:"module"`
	// module version (latest if empty)
	Version string `form:"version"`
	ListParams
}

// ModuleParams are query parameters for /v1beta/module/{path}.
type ModuleParams struct {
	// module version (latest if empty)
	Version string `form:"version"`
	// Whether to include licenses in the result.
	Licenses bool `form:"licenses"`
	// Whether to include the README in the result.
	Readme bool `form:"readme"`
}

// VersionsParams are query parameters for /v1beta/versions/{path}.
type VersionsParams struct {
	ListParams
}

// PackagesParams are query parameters for /v1beta/packages/{path}.
type PackagesParams struct {
	// module version (latest if empty)
	Version string `form:"version"`
	ListParams
}

// SearchParams are query parameters for /v1beta/search.
type SearchParams struct {
	// Find packages matching this query.
	Query string `form:"q"`
	// If non-empty, find symbols matching this string.
	// The query further restricts the search to matching packages.
	Symbol string `form:"symbol"`
	ListParams
}

// VulnParams are query parameters for /v1beta/vulns/{module}.
type VulnParams struct {
	// module version (latest if empty)
	Version string `form:"version"`
	ListParams
}

// ParseParams populates a struct from url.Values using 'form' tags.
// dst must be a pointer to a struct. It supports embedded structs recursively,
// pointers, slices, and basic types (string, bool, int types).
func ParseParams(v url.Values, dst any) error {
	val := reflect.ValueOf(dst)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return InternalServerError("dst must be a pointer to a struct")
	}
	if err := parseValue(v, val.Elem()); err != nil {
		return BadRequest(err.Error(), "correct the query parameter and retry")
	}
	return nil
}

func parseValue(v url.Values, val reflect.Value) error {
	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		structField := typ.Field(i)

		if !field.CanSet() {
			continue
		}

		if structField.Anonymous {
			f := field
			if f.Kind() == reflect.Pointer {
				if f.IsNil() {
					if !f.CanSet() {
						continue
					}
					f.Set(reflect.New(f.Type().Elem()))
				}
				f = f.Elem()
			}
			if f.Kind() == reflect.Struct {
				if err := parseValue(v, f); err != nil {
					return err
				}
				continue
			}
		}

		tag := structField.Tag.Get("form")
		if tag == "" {
			continue
		}

		if !v.Has(tag) {
			continue
		}

		if err := setField(field, tag, v[tag]); err != nil {
			return err
		}
	}
	return nil
}

func setField(field reflect.Value, tag string, vals []string) error {
	if len(vals) == 0 {
		return nil
	}

	if field.Kind() == reflect.Slice {
		slice := reflect.MakeSlice(field.Type(), len(vals), len(vals))
		for i, v := range vals {
			if err := setAny(slice.Index(i), tag, v); err != nil {
				return err
			}
		}
		field.Set(slice)
		return nil
	}

	return setAny(field, tag, vals[0])
}

func setAny(field reflect.Value, tag, val string) error {
	if field.Kind() != reflect.Pointer {
		return setSingle(field, tag, val)
	}
	if field.IsNil() {
		field.Set(reflect.New(field.Type().Elem()))
	}
	return setAny(field.Elem(), tag, val)
}

func setSingle(field reflect.Value, tag, val string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val == "" {
			return fmt.Errorf("empty value for %s", tag)
		}
		iv, err := strconv.ParseInt(val, 10, field.Type().Bits())
		if err != nil {
			return fmt.Errorf("invalid value %q for %s: %w", val, tag, err)
		}
		field.SetInt(iv)
	case reflect.Bool:
		if val == "" {
			field.SetBool(false)
			return nil
		}
		bv, err := strconv.ParseBool(val)
		if err != nil {
			return fmt.Errorf("invalid boolean value %q for %s: %w", val, tag, err)
		}
		field.SetBool(bv)
	default:
		return fmt.Errorf("unsupported type %s for field %s", field.Type(), tag)
	}
	return nil
}

// pageParams resolves the limit and offset from ListParams.
// It returns an error if the token is invalid.
func (lp ListParams) pageParams(defaultLimit int) (limit, offset int, err error) {
	limit = lp.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if lp.Token != "" {
		offset, err = decodePageToken(lp.Token)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid next-page token: %v", err)
		}
	}
	return limit, offset, nil
}
