// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package fetch

import "golang.org/x/mod/module"

// knownAlternatives lists module paths that are known to be forks of other
// modules.
// For example, github.com/msopentech/azure-sdk-for-go
// is an alternative to github.com/Azure/azure-sdk-for-go.
// Map keys are case-sensitive and should not include a final major version
// like "/v3" or ".v3" for gopkg.in paths.
//
// When a module has a go.mod file, we can detect alternatives by comparing the
// module path with the path in the go.mod file. This list is for modules
// without go.mod files.
var knownAlternatives = map[string]string{
	"github.com/Azure/Azure-sdk-for-go":                "github.com/Azure/azure-sdk-for-go",
	"github.com/azure/azure-sdk-for-go":                "github.com/Azure/azure-sdk-for-go",
	"github.com/evenh/azure-sdk-for-go":                "github.com/Azure/azure-sdk-for-go",
	"github.com/msopentech/azure-sdk-for-go":           "github.com/Azure/azure-sdk-for-go",
	"github.com/MSOpenTech/azure-sdk-for-go":           "github.com/Azure/azure-sdk-for-go",
	"github.com/scott-the-programmer/azure-sdk-for-go": "github.com/Azure/azure-sdk-for-go",
	"gopkg.in/Azure/azure-sdk-for-go":                  "github.com/Azure/azure-sdk-for-go",
	"gopkg.in/azure/azure-sdk-for-go":                  "github.com/Azure/azure-sdk-for-go",
	"github.com/masslessparticle/azure-sdk-for-go":     "github.com/Azure/azure-sdk-for-go",
	"github.com/aliyun/alibaba-cloud-sdk-go":           "github.com/Azure/azure-sdk-for-go",
	"github.com/johnstairs/azure-sdk-for-go":           "github.com/Azure/azure-sdk-for-go",
	"github.com/shopify/sarama":                        "github.com/Shopify/sarama",
}

// knownAlternativeFor returns the module that the given module path is an alternative to,
// or the empty string if there is no such module.
//
// It consults the knownAlternatives map, ignoring version suffixes.
func knownAlternativeFor(modulePath string) string {
	key, _, ok := module.SplitPathVersion(modulePath)
	if !ok {
		return ""
	}
	return knownAlternatives[key]
}
