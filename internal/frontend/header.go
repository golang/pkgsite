// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"path"
	"strings"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/stdlib"
)

const (
	pageTypeModule    = "module"
	pageTypeDirectory = "directory"
	pageTypePackage   = "package"
	pageTypeCommand   = "command"
	pageTypeModuleStd = "std"
	pageTypeStdlib    = "standard library"
)

// pageTitle determines the pageTitles for a given unit.
// See TestPageTitlesAndTypes for examples.
func pageTitle(um *internal.UnitMeta) string {
	switch {
	case um.Path == stdlib.ModulePath:
		return "Standard library"
	case um.IsCommand():
		return effectiveName(um.Path, um.Name)
	case um.IsPackage():
		return um.Name
	case um.IsModule():
		return path.Base(um.Path)
	default:
		return path.Base(um.Path) + "/"
	}
}

// pageType determines the pageType for a given unit.
func pageType(um *internal.UnitMeta) string {
	if um.Path == stdlib.ModulePath {
		return pageTypeModuleStd
	}
	if um.IsCommand() {
		return pageTypeCommand
	}
	if um.IsPackage() {
		return pageTypePackage
	}
	if um.IsModule() {
		return pageTypeModule
	}
	return pageTypeDirectory
}

// pageLabels determines the labels to display for a given unit.
// See TestPageTitlesAndTypes for examples.
func pageLabels(um *internal.UnitMeta) []string {
	var pageTypes []string
	if um.Path == stdlib.ModulePath {
		return nil
	}
	if um.IsCommand() {
		pageTypes = append(pageTypes, pageTypeCommand)
	} else if um.IsPackage() {
		pageTypes = append(pageTypes, pageTypePackage)
	}
	if um.IsModule() {
		pageTypes = append(pageTypes, pageTypeModule)
	}
	if !um.IsPackage() && !um.IsModule() {
		pageTypes = append(pageTypes, pageTypeDirectory)
	}
	if stdlib.Contains(um.Path) {
		pageTypes = append(pageTypes, pageTypeStdlib)
	}
	return pageTypes
}

// effectiveName returns either the command name or package name.
func effectiveName(pkgPath, pkgName string) string {
	if pkgName != "main" {
		return pkgName
	}
	var prefix string // package path without version
	if pkgPath[len(pkgPath)-3:] == "/v1" {
		prefix = pkgPath[:len(pkgPath)-3]
	} else {
		prefix, _, _ = module.SplitPathVersion(pkgPath)
	}
	_, base := path.Split(prefix)
	return base
}

func constructPackageURL(pkgPath, modulePath, linkVersion string) string {
	if linkVersion == internal.LatestVersion {
		return "/" + pkgPath
	}
	if pkgPath == modulePath || modulePath == stdlib.ModulePath {
		return fmt.Sprintf("/%s@%s", pkgPath, linkVersion)
	}
	return fmt.Sprintf("/%s@%s/%s", modulePath, linkVersion, strings.TrimPrefix(pkgPath, modulePath+"/"))
}

// absoluteTime takes a date and returns returns a human-readable,
// date with the format mmm d, yyyy:
func absoluteTime(date time.Time) string {
	return date.Format("Jan _2, 2006")
}
