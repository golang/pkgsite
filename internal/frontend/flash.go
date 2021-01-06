// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/pkgsite/internal/derrors"
)

// alternativeModuleFlash indicates the alternative module path that
// a request was redirected from.
const alternativeModuleFlash = "tmp-redirected-from-alternative-module"

func getFlashMessage(w http.ResponseWriter, r *http.Request, name string) (_ string, err error) {
	defer derrors.Wrap(&err, "getFlashMessage")
	c, err := r.Cookie(name)
	if err != nil && err != http.ErrNoCookie {
		return "", fmt.Errorf("r.Cookie(%q): %v", name, err)
	}
	if c == nil {
		return "", nil
	}
	val, err := base64.URLEncoding.DecodeString(c.Value)
	if err != nil {
		return "", nil
	}
	http.SetCookie(w, &http.Cookie{
		Name:    name,
		Expires: time.Now().Add(-1 * time.Minute),
	})
	return string(val), nil
}

func setFlashMessage(w http.ResponseWriter, name, val, urlPath string) {
	value := base64.URLEncoding.EncodeToString([]byte(val))
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: urlPath})
}
