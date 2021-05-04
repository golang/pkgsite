// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import "golang.org/x/pkgsite/internal/frontend"

var versionsPageMultiGoosDuplicates = []*frontend.VersionList{
	{
		VersionListKey: frontend.VersionListKey{
			ModulePath: "example.com/symbols",
			Major:      "v1",
		},
		Versions: []*frontend.VersionSummary{
			{
				CommitTime:          "Jan 30, 2019",
				Link:                "/example.com/symbols@v1.2.0/duplicate",
				Retracted:           false,
				RetractionRationale: "",
				Version:             "v1.2.0",
				IsMinor:             true,
				Symbols: [][]*frontend.Symbol{
					{
						{
							Name:     "TokenType",
							Synopsis: "const TokenType",
							Link:     "/example.com/symbols@v1.2.0/duplicate?GOOS=windows#TokenType",
							New:      true,
							Section:  "Constants",
							Kind:     "Constant",
							Builds:   []string{"windows/amd64"},
						},
					},
					{
						{
							Name:     "TokenType",
							Synopsis: "type TokenType int",
							Link:     "/example.com/symbols@v1.2.0/duplicate?GOOS=darwin#TokenType",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
							Builds:   []string{"darwin/amd64", "linux/amd64"},
							// Children is nil because TokenShort was first
							// introduced at an earlier version.
							// Its parent and section changed at this version,
							// but we don't surface that information.
						},
						{
							Name:     "TokenType",
							Synopsis: "type TokenType struct",
							Link:     "/example.com/symbols@v1.2.0/duplicate?GOOS=js#TokenType",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
							Children: []*frontend.Symbol{
								{
									Name:     "TokenShort",
									Synopsis: "func TokenShort() TokenType",
									Link:     "/example.com/symbols@v1.2.0/duplicate?GOOS=js#TokenShort",
									New:      true,
									Section:  "Types",
									Kind:     "Function",
								},
							},
							Builds: []string{"js/wasm"},
						},
					},
				},
			},
			{
				CommitTime:          "Jan 30, 2019",
				Link:                "/example.com/symbols@v1.1.0/duplicate",
				Retracted:           false,
				RetractionRationale: "",
				Version:             "v1.1.0",
				IsMinor:             true,
				Symbols: [][]*frontend.Symbol{
					{
						{
							Name:     "TokenShort",
							Synopsis: "const TokenShort",
							Link:     "/example.com/symbols@v1.1.0/duplicate?GOOS=darwin#TokenShort",
							New:      true,
							Section:  "Constants",
							Kind:     "Constant",
							Builds:   []string{"darwin/amd64", "linux/amd64"},
						},
					},
				},
			},
		},
	},
}

var versionsPageMultiGoos = []*frontend.VersionList{
	{
		VersionListKey: frontend.VersionListKey{
			ModulePath: "example.com/symbols",
			Major:      "v1",
		},
		Versions: []*frontend.VersionSummary{
			{
				CommitTime:          "Jan 30, 2019",
				Link:                "/example.com/symbols@v1.2.0/multigoos",
				Retracted:           false,
				RetractionRationale: "",
				Version:             "v1.2.0",
				IsMinor:             true,
				Symbols: [][]*frontend.Symbol{
					{
						{
							Name:     "CloseOnExec",
							Synopsis: "func CloseOnExec(n int)",
							Link:     "/example.com/symbols@v1.2.0/multigoos?GOOS=js#CloseOnExec",
							New:      true,
							Section:  "Functions",
							Kind:     "Function",
							Builds:   []string{"js/wasm"},
						},
					},
				},
			},
			{
				CommitTime:          "Jan 30, 2019",
				Link:                "/example.com/symbols@v1.1.0/multigoos",
				Retracted:           false,
				RetractionRationale: "",
				Version:             "v1.1.0",
				IsMinor:             true,
				Symbols: [][]*frontend.Symbol{
					{
						{
							Name:     "CloseOnExec",
							Synopsis: "func CloseOnExec(foo string) error",
							Link:     "/example.com/symbols@v1.1.0/multigoos?GOOS=windows#CloseOnExec",
							New:      true,
							Section:  "Functions",
							Kind:     "Function",
							Builds:   []string{"windows/amd64"},
						},
						{
							Name:     "CloseOnExec",
							Synopsis: "func CloseOnExec(num int) (int, error)",
							Link:     "/example.com/symbols@v1.1.0/multigoos?GOOS=darwin#CloseOnExec",
							New:      true,
							Section:  "Functions",
							Kind:     "Function",
							Builds:   []string{"darwin/amd64", "linux/amd64"},
						},
					},
				},
			},
		},
	},
}

var versionsPageHello = []*frontend.VersionList{
	{
		VersionListKey: frontend.VersionListKey{
			ModulePath: "example.com/symbols",
			Major:      "v1",
		},
		Versions: []*frontend.VersionSummary{
			{
				CommitTime:          "Jan 30, 2019",
				Link:                "/example.com/symbols@v1.2.0/hello",
				Retracted:           false,
				RetractionRationale: "",
				Version:             "v1.2.0",
				IsMinor:             true,
				Symbols: [][]*frontend.Symbol{
					{
						{
							Name:     "Hello",
							Synopsis: "func Hello() string",
							Link:     "/example.com/symbols@v1.2.0/hello?GOOS=js#Hello",
							New:      true,
							Section:  "Functions",
							Kind:     "Function",
							Builds:   []string{"js/wasm", "windows/amd64"},
						},
					},
				},
			},
			{
				CommitTime:          "Jan 30, 2019",
				Link:                "/example.com/symbols@v1.1.0/hello",
				Retracted:           false,
				RetractionRationale: "",
				Version:             "v1.1.0",
				IsMinor:             true,
				Symbols: [][]*frontend.Symbol{
					{
						{
							Name:     "Hello",
							Synopsis: "func Hello() string",
							Link:     "/example.com/symbols@v1.1.0/hello?GOOS=darwin#Hello",
							New:      true,
							Section:  "Functions",
							Kind:     "Function",
							Builds:   []string{"darwin/amd64", "linux/amd64"},
						},
						{
							Name:     "HelloJS",
							Synopsis: "func HelloJS() string",
							Link:     "/example.com/symbols@v1.1.0/hello?GOOS=js#HelloJS",
							New:      true,
							Section:  "Functions",
							Kind:     "Function",
							Builds:   []string{"js/wasm"},
						},
					},
				},
			},
		},
	},
}

var versionsPageSymbols = []*frontend.VersionList{
	{
		VersionListKey: frontend.VersionListKey{
			ModulePath: "example.com/symbols",
			Major:      "v1",
		},
		Versions: []*frontend.VersionSummary{
			{
				CommitTime: "Jan 30, 2019",
				Link:       "/example.com/symbols@v1.2.0",
				Version:    "v1.2.0",
				IsMinor:    true,
			},
			{
				CommitTime: "Jan 30, 2019",
				Link:       "/example.com/symbols@v1.1.0",
				Version:    "v1.1.0",
				IsMinor:    true,
				Symbols: [][]*frontend.Symbol{
					{
						{
							Name:     "I2",
							Synopsis: "type I2",
							Link:     "/example.com/symbols@v1.1.0#I2",
							Section:  "Types",
							Kind:     "Type",
							Children: []*frontend.Symbol{
								{
									Name:     "I2.M2",
									Synopsis: "M2 func()",
									Link:     "/example.com/symbols@v1.1.0#I2.M2",
									New:      true,
									Section:  "Types",
									Kind:     "Method",
								},
							},
						},
						{
							Name:     "S2",
							Synopsis: "type S2",
							Link:     "/example.com/symbols@v1.1.0#S2",
							Section:  "Types",
							Kind:     "Type",
							Children: []*frontend.Symbol{
								{
									Name:     "S2.G",
									Synopsis: "G int",
									Link:     "/example.com/symbols@v1.1.0#S2.G",
									New:      true,
									Section:  "Types",
									Kind:     "Field",
								},
							},
						},
						{
							Name:     "String",
							Synopsis: "type String bool",
							Link:     "/example.com/symbols@v1.1.0#String",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
						},
					},
				},
			},
			{
				CommitTime:          "Jan 30, 2019",
				Link:                "/example.com/symbols@v1.0.0",
				Retracted:           false,
				RetractionRationale: "",
				Version:             "v1.0.0",
				IsMinor:             true,
				Symbols: [][]*frontend.Symbol{
					{
						{
							Name:     "AA",
							Synopsis: "const AA",
							Link:     "/example.com/symbols@v1.0.0#AA",
							New:      true,
							Section:  "Constants",
							Kind:     "Constant",
						},
						{
							Name:     "BB",
							Synopsis: "const BB",
							Link:     "/example.com/symbols@v1.0.0#BB",
							New:      true,
							Section:  "Constants",
							Kind:     "Constant",
						},
						{
							Name:     "C",
							Synopsis: "const C",
							Link:     "/example.com/symbols@v1.0.0#C",
							New:      true,
							Section:  "Constants",
							Kind:     "Constant",
						},
						{
							Name:     "CC",
							Synopsis: "const CC",
							Link:     "/example.com/symbols@v1.0.0#CC",
							New:      true,
							Section:  "Constants",
							Kind:     "Constant",
						},
					},
					{
						{
							Name:     "A",
							Synopsis: "var A string",
							Link:     "/example.com/symbols@v1.0.0#A",
							New:      true,
							Section:  "Variables",
							Kind:     "Variable",
						},
						{
							Name:     "B",
							Synopsis: "var B string",
							Link:     "/example.com/symbols@v1.0.0#B",
							New:      true,
							Section:  "Variables",
							Kind:     "Variable",
						},
						{

							Name:     "ErrA",
							Synopsis: `var ErrA = errors.New("error A")`,
							Link:     "/example.com/symbols@v1.0.0#ErrA",
							New:      true,
							Section:  "Variables",
							Kind:     "Variable",
						},
						{

							Name:     "ErrB",
							Synopsis: `var ErrB = errors.New("error B")`,
							Link:     "/example.com/symbols@v1.0.0#ErrB",
							New:      true,
							Section:  "Variables",
							Kind:     "Variable",
						},
						{
							Name:     "V",
							Synopsis: "var V = 2",
							Link:     "/example.com/symbols@v1.0.0#V",
							New:      true,
							Section:  "Variables",
							Kind:     "Variable",
						},
					},
					{
						{
							Name:     "F",
							Synopsis: "func F()",
							Link:     "/example.com/symbols@v1.0.0#F",
							New:      true,
							Section:  "Functions",
							Kind:     "Function",
						},
					},
					{
						{
							Name:     "I1",
							Synopsis: "type I1 interface",
							Link:     "/example.com/symbols@v1.0.0#I1",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
							Children: []*frontend.Symbol{
								{
									Name:     "I1.M1",
									Synopsis: "M1 func()",
									Link:     "/example.com/symbols@v1.0.0#I1.M1",
									New:      true,
									Section:  "Types",
									Kind:     "Method",
								},
							},
						},
						{
							Name:     "I2",
							Synopsis: "type I2 interface",
							Link:     "/example.com/symbols@v1.0.0#I2",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
						},
						{
							Name:     "Int",
							Synopsis: "type Int int",
							Link:     "/example.com/symbols@v1.0.0#Int",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
						},
						{
							Name:     "Num",
							Synopsis: "type Num int",
							Link:     "/example.com/symbols@v1.0.0#Num",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
							Children: []*frontend.Symbol{
								{
									Name:     "DD",
									Synopsis: "const DD",
									Link:     "/example.com/symbols@v1.0.0#DD",
									New:      true,
									Section:  "Types",
									Kind:     "Constant",
								},
								{
									Name:     "EE",
									Synopsis: "const EE",
									Link:     "/example.com/symbols@v1.0.0#EE",
									New:      true,
									Section:  "Types",
									Kind:     "Constant",
								},
								{
									Name:     "FF",
									Synopsis: "const FF",
									Link:     "/example.com/symbols@v1.0.0#FF",
									New:      true,
									Section:  "Types",
									Kind:     "Constant",
								},
							},
						},
						{
							Name:     "S1",
							Synopsis: "type S1 struct",
							Link:     "/example.com/symbols@v1.0.0#S1",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
							Children: []*frontend.Symbol{
								{
									Name:     "S1.F",
									Synopsis: "F int",
									Link:     "/example.com/symbols@v1.0.0#S1.F",
									New:      true,
									Section:  "Types",
									Kind:     "Field",
								},
							},
						},
						{
							Name:     "S2",
							Synopsis: "type S2 struct",
							Link:     "/example.com/symbols@v1.0.0#S2",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
						},
						{
							Name:     "T",
							Synopsis: "type T int",
							Link:     "/example.com/symbols@v1.0.0#T",
							New:      true,
							Section:  "Types",
							Kind:     "Type",
							Children: []*frontend.Symbol{
								{
									Name:     "CT",
									Synopsis: "const CT",
									Link:     "/example.com/symbols@v1.0.0#CT",
									New:      true,
									Section:  "Types",
									Kind:     "Constant",
								},
								{
									Name:     "VT",
									Synopsis: "var VT T",
									Link:     "/example.com/symbols@v1.0.0#VT",
									New:      true,
									Section:  "Types",
									Kind:     "Variable",
								},
								{
									Name:     "TF",
									Synopsis: "func TF() T",
									Link:     "/example.com/symbols@v1.0.0#TF",
									New:      true,
									Section:  "Types",
									Kind:     "Function",
								},
								{
									Name:     "T.M",
									Synopsis: "func (T) M()",
									Link:     "/example.com/symbols@v1.0.0#T.M",
									New:      true,
									Section:  "Types",
									Kind:     "Method",
								},
							},
						},
					},
				},
			},
		},
	},
}
