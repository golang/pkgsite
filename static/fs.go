// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package static

import "embed"

// Until https://golang.org/issue/43854 is implemented, all directories
// containing files beginning with an underscore must be specified explicitly.

//go:embed doc/* frontend/* shared/* worker/*
//go:embed frontend/unit/* frontend/unit/main/*
var FS embed.FS
