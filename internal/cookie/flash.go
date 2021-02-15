// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cookie is used to get and set HTTP cookies.
package cookie

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/pkgsite/internal/derrors"
)

// AlternativeModuleFlash indicates the alternative module path that
// a request was redirected from.
const AlternativeModuleFlash = "tmp-redirected-from-alternative-module"

// Extract returns the value of the cookie at name and deletes the cookie.
func Extract(w http.ResponseWriter, r *http.Request, name string) (_ string, err error) {
	defer derrors.Wrap(&err, "Extract")
	c, err := r.Cookie(name)
	if err != nil && err != http.ErrNoCookie {
		return "", fmt.Errorf("r.Cookie(%q): %v", name, err)
	}
	if c == nil {
		return "", nil
	}
	val, err := Base64Value(c)
	if err != nil {
		return "", nil
	}
	http.SetCookie(w, &http.Cookie{
		Name:    name,
		Path:    r.URL.Path,
		Expires: time.Unix(0, 0),
	})
	return val, nil
}

// Base64Value decodes  the value of c using the Base64 URL encoding and returns it as a string.
func Base64Value(c *http.Cookie) (string, error) {
	val, err := base64.URLEncoding.DecodeString(c.Value)
	if err != nil {
		return "", err
	}
	return string(val), nil
}

// Set sets a cookie at the urlPath with name and val.
func Set(w http.ResponseWriter, name, val, urlPath string) {
	value := base64.URLEncoding.EncodeToString([]byte(val))
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: urlPath})
}
