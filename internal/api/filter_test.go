// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"go/ast"
	"go/parser"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func parseExpr(t *testing.T, s string) ast.Expr {
	t.Helper()
	expr, err := parser.ParseExpr(s)
	if err != nil {
		t.Fatalf("ParseExpr(%q) failed: %v", s, err)
	}
	return expr
}

func TestEvalBase(t *testing.T) {
	env := map[string]any{
		"x": 10,
		"s": "hello",
		"identity": func(v any) any {
			return v
		},
		"add1": func(v int) int {
			return v + 1
		},
		"greet": func(name string) (string, error) {
			if name == "" {
				return "", errors.New("empty name")
			}
			return "Hello " + name, nil
		},
		"panicFunc": func() {
			panic("intentional panic")
		},
	}

	tests := []struct {
		expr    string
		want    any
		wantErr bool
	}{
		// Literals
		{expr: `1`, want: 1},
		{expr: `"hello"`, want: "hello"},
		{expr: `true`, want: true},
		{expr: `false`, want: false},
		{expr: `nil`, want: nil},

		// Variables
		{expr: `x`, want: 10},
		{expr: `s`, want: "hello"},
		{expr: `y`, wantErr: true}, // undefined

		// Arithmetic operators
		{expr: `1 + 2`, want: 3},
		{expr: `5 - 3`, want: 2},
		{expr: `2 * 3`, want: 6},
		{expr: `6 / 2`, want: 3},
		{expr: `5 % 2`, want: 1},
		{expr: `x + 5`, want: 15},
		{expr: `-5`, want: -5},
		{expr: `-x`, want: -10},

		// String concatenation
		{expr: `s + " world"`, want: "hello world"},
		{expr: `"a" + "b"`, want: "ab"},

		// Arithmetic errors
		{expr: `6 / 0`, wantErr: true},   // division by zero
		{expr: `5 % 0`, wantErr: true},   // division by zero
		{expr: `x + s`, wantErr: true},   // type mismatch (int + string)
		{expr: `s - "a"`, wantErr: true}, // invalid op for string
		{expr: `-s`, wantErr: true},      // invalid unary minus for string

		// Comparison operators (int)
		{expr: `1 < 2`, want: true},
		{expr: `2 < 1`, want: false},
		{expr: `1 < 1`, want: false},
		{expr: `1 > 2`, want: false},
		{expr: `2 > 1`, want: true},
		{expr: `1 > 1`, want: false},
		{expr: `1 <= 2`, want: true},
		{expr: `2 <= 1`, want: false},
		{expr: `1 <= 1`, want: true},
		{expr: `1 >= 2`, want: false},
		{expr: `2 >= 1`, want: true},
		{expr: `1 >= 1`, want: true},
		{expr: `1 == 1`, want: true},
		{expr: `1 == 2`, want: false},
		{expr: `1 != 1`, want: false},
		{expr: `1 != 2`, want: true},

		// Comparison operators (string)
		{expr: `"a" < "b"`, want: true},
		{expr: `"b" < "a"`, want: false},
		{expr: `"a" == "a"`, want: true},
		{expr: `"a" == "b"`, want: false},
		{expr: `"a" != "b"`, want: true},

		// Comparison operators (bool)
		{expr: `true == true`, want: true},
		{expr: `true == false`, want: false},
		{expr: `true != false`, want: true},

		// Comparison with nil
		{expr: `nil == nil`, want: true},
		{expr: `nil != nil`, want: false},
		{expr: `identity(nil) == nil`, want: true},
		{expr: `identity(1) == nil`, want: false},
		{expr: `s == nil`, want: false},

		// Comparison errors
		{expr: `1 < "a"`, wantErr: true},      // type mismatch
		{expr: `true == 1`, want: false},      // type mismatch
		{expr: `true < false`, wantErr: true}, // invalid op for bool

		// Logical operators
		{expr: `true && true`, want: true},
		{expr: `true && false`, want: false},
		{expr: `false && true`, want: false},
		{expr: `false && false`, want: false},
		{expr: `true || true`, want: true},
		{expr: `true || false`, want: true},
		{expr: `false || true`, want: true},
		{expr: `false || false`, want: false},
		{expr: `!true`, want: false},
		{expr: `!false`, want: true},

		// Short-circuiting verification
		{expr: `false && panicFunc()`, want: false},     // short-circuit &&
		{expr: `true || panicFunc()`, want: true},       // short-circuit ||
		{expr: `false && greet("") == ""`, want: false}, // short-circuit && with error-returning func

		// Logical errors
		{expr: `true && 1`, wantErr: true},    // type mismatch
		{expr: `false || "a"`, wantErr: true}, // type mismatch
		{expr: `!1`, wantErr: true},           // invalid type for !

		// Function calls
		{expr: `identity(1)`, want: 1},
		{expr: `identity("a")`, want: "a"},
		{expr: `add1(x)`, want: 11},
		{expr: `greet(s)`, want: "Hello hello"},
		{expr: `greet("")`, wantErr: true},   // error return
		{expr: `panicFunc()`, wantErr: true}, // panic recovery
		{expr: `identity()`, wantErr: true},  // arg mismatch (0 instead of 1)
		{expr: `x()`, wantErr: true},         // not a function
	}

	for _, tc := range tests {
		t.Run(tc.expr, func(t *testing.T) {
			astExpr := parseExpr(t, tc.expr)
			got, err := evaluate(astExpr, env)
			if (err != nil) != tc.wantErr {
				t.Errorf("evaluate(%s) error = %v, wantErr %v", tc.expr, err, tc.wantErr)
				return
			}
			if tc.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("evaluate(%s) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestEvalComprehensive(t *testing.T) {
	env := map[string]any{
		"x": 10,
		"s": "hello",
		"greet": func(name string) (string, error) {
			return "Hello " + name, nil
		},
	}

	exprStr := `x > 5 && greet(s) == "Hello hello" && (1 + 2 * 3) == 7`
	astExpr := parseExpr(t, exprStr)
	got, err := evaluate(astExpr, env)
	if err != nil {
		t.Fatalf("evaluate(%s) failed: %v", exprStr, err)
	}

	want := true
	if got != want {
		t.Errorf("evaluate(%s) = %v, want %v", exprStr, got, want)
	}
}

func TestEvalFilter(t *testing.T) {
	sym := Symbol{
		Name:     "Eval",
		Kind:     "func",
		Synopsis: "evaluates expressions",
		Parent:   "",
	}

	tests := []struct {
		filter  string
		want    bool
		wantErr bool
	}{
		{filter: `name == "Eval"`, want: true},
		{filter: `kind == "func"`, want: true},
		{filter: `parent == ""`, want: true},
		{filter: `kind == "func" && name == "Eval"`, want: true},
		{filter: `kind == "var" || name == "Eval"`, want: true},
		{filter: `kind == "func" && parent != ""`, want: false},
		{filter: `name == "Evaluate"`, want: false},
		{filter: `synopsis == "evaluates expressions"`, want: true},

		{filter: `name == 1`, want: false}, // different types == is false
		{filter: `name != 1`, want: true},  // different types != is true

		// contains and matches
		{filter: `contains(synopsis, "eval")`, want: true},
		{filter: `contains(synopsis, "invalid")`, want: false},
		{filter: `matches(name, "^Ev")`, want: true},
		{filter: `matches(name, "val$")`, want: true},
		{filter: "matches(name, `^a`)", want: false},

		// Errors
		{filter: `kind < 1`, wantErr: true},                 // type mismatch for <
		{filter: `nonexistent == ""`, wantErr: true},        // undefined identifier
		{filter: `matches(name,"[invalid")`, wantErr: true}, // invalid regex
	}

	for _, tc := range tests {
		t.Run(tc.filter, func(t *testing.T) {
			list := []Symbol{sym}
			got, err := filterStruct(list, tc.filter)
			if (err != nil) != tc.wantErr {
				t.Errorf("filter2(%+v, %q) error = %v, wantErr %v", list, tc.filter, err, tc.wantErr)
				return
			}
			if tc.wantErr {
				return
			}
			if tc.want {
				if len(got) != 1 || got[0] != sym {
					t.Errorf("filter2(%+v, %q) = %v, want [%+v]", list, tc.filter, got, sym)
				}
			} else {
				if len(got) != 0 {
					t.Errorf("filter2(%+v, %q) = %v, want empty", list, tc.filter, got)
				}
			}
		})
	}
}

func TestFilterErrors(t *testing.T) {
	type badFuncStruct struct {
		Func func() (int, int)
	}
	type manyFuncStruct struct {
		Func func() (int, int, int)
	}
	type variadicFuncStruct struct {
		Func func(...int) bool
	}
	type intStruct struct {
		X int
	}
	type panicFuncStruct struct {
		Func func() bool
	}

	tests := []struct {
		name       string
		run        func() error
		wantBad    bool   // true if we expect a BadRequest error (*Error)
		wantSubstr string // substring in error message
	}{
		{
			name: "filterString empty varName",
			run: func() error {
				_, err := filterString([]string{"a"}, "true", "")
				return err
			},
			wantBad:    false,
			wantSubstr: "string filter must have varName",
		},
		{
			name: "filterStruct non-struct",
			run: func() error {
				_, err := filterStruct([]int{1}, "true")
				return err
			},
			wantBad:    false,
			wantSubstr: "need struct or pointer to struct",
		},
		{
			name: "parse error",
			run: func() error {
				_, err := filterString([]string{"a"}, "invalid go expr", "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "parsing filter",
		},
		{
			name: "eval error undefined identifier",
			run: func() error {
				_, err := filterString([]string{"a"}, "unknown_var == 'a'", "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "undefined identifier",
		},
		{
			name: "eval error regex compile",
			run: func() error {
				_, err := filterString([]string{"a"}, `matches(x, "[invalid")`, "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "parsing regexp",
		},
		{
			name: "eval error arg count mismatch",
			run: func() error {
				_, err := filterString([]string{"a"}, "contains(x)", "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "argument count mismatch",
		},
		{
			name: "eval error not a function",
			run: func() error {
				_, err := filterString([]string{"a"}, "x()", "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "not a function",
		},
		{
			name: "eval error division by zero",
			run: func() error {
				_, err := filterString([]string{"a"}, "1/0 == 1", "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "division by zero",
		},
		{
			name: "eval error invalid type for +",
			run: func() error {
				_, err := filterString([]string{"a"}, `(x == "a") + 1`, "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "invalid type for +",
		},
		{
			name: "non-bool result",
			run: func() error {
				_, err := filterString([]string{"a"}, "1 + 1", "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "did not evaluate to bool",
		},
		{
			name: "eval error second return value must be error",
			run: func() error {
				list := []badFuncStruct{{Func: func() (int, int) { return 1, 2 }}}
				_, err := filterStruct(list, "Func() == 1")
				return err
			},
			wantBad:    true,
			wantSubstr: "second return value must be error, got int",
		},
		{
			name: "eval error function has too many return values",
			run: func() error {
				list := []manyFuncStruct{{Func: func() (int, int, int) { return 1, 2, 3 }}}
				_, err := filterStruct(list, "Func() == 1")
				return err
			},
			wantBad:    true,
			wantSubstr: "function has too many return values: 3",
		},
		{
			name: "eval error variadic functions are not supported",
			run: func() error {
				list := []variadicFuncStruct{{Func: func(x ...int) bool { return true }}}
				_, err := filterStruct(list, "Func(1, 2)")
				return err
			},
			wantBad:    true,
			wantSubstr: "variadic functions are not supported",
		},
		{
			name: "eval error unsupported basic lit kind",
			run: func() error {
				_, err := filterString([]string{"a"}, "'a' == 'a'", "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "unsupported basic lit kind",
		},
		{
			name: "eval error unsupported unary operator",
			run: func() error {
				list := []intStruct{{X: 1}}
				_, err := filterStruct(list, "^X == 1")
				return err
			},
			wantBad:    true,
			wantSubstr: "unsupported unary operator",
		},
		{
			name: "eval error invalid type for <",
			run: func() error {
				_, err := filterString([]string{"a"}, `(x == "a") < 1`, "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "invalid type for <",
		},
		{
			name: "eval error unsupported binary operator",
			run: func() error {
				list := []intStruct{{X: 1}}
				_, err := filterStruct(list, "(X & 1) == 1")
				return err
			},
			wantBad:    true,
			wantSubstr: "unsupported binary operator",
		},
		{
			name: "eval error expected type T, got T",
			run: func() error {
				_, err := filterString([]string{"a"}, "!x", "x")
				return err
			},
			wantBad:    true,
			wantSubstr: "expected type bool, got string",
		},
		{
			name: "eval error panic during function call",
			run: func() error {
				list := []panicFuncStruct{{Func: func() bool { panic("aaaaa") }}}
				_, err := filterStruct(list, "Func()")
				return err
			},
			wantBad:    true,
			wantSubstr: "panic during function call: aaaaa",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var apiErr *Error
			isBad := errors.As(err, &apiErr)

			if tc.wantBad {
				if !isBad {
					t.Errorf("expected BadRequest (*Error), got %T (%v)", err, err)
				} else if apiErr.Code != http.StatusBadRequest {
					t.Errorf("expected Code %d, got %d", http.StatusBadRequest, apiErr.Code)
				}
			} else {
				if isBad {
					t.Errorf("expected plain Go error, got BadRequest (*Error): %v", err)
				}
			}

			if tc.wantSubstr != "" {
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Errorf("expected error to contain %q, got %q", tc.wantSubstr, err.Error())
				}
			}
		})
	}
}
