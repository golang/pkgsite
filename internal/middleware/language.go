// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"context"
	"net/http"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type tagKey struct{}

var matcher = language.NewMatcher(message.DefaultCatalog.Languages())

// Language is a middleware that provides browser i18n information to handlers,
// in the form of a golang.org/x/text/language.Tag.
func Language(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tag, _ := language.MatchStrings(matcher, r.Header.Get("Accept-Language"))
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tagKey{}, tag)))
	})
}

// LanguageTag returns the language.Tag from the context, or language.English if none is set.
func LanguageTag(ctx context.Context) language.Tag {
	if tag, ok := ctx.Value(tagKey{}).(language.Tag); ok {
		return tag
	}
	return language.English
}
