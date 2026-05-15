// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file features an evaluator for the subset of Go that constitutes
// filter expressions. To simplify the code, the internal functions of the
// evaluator panic on error, and the entry point, the evaluate function,
// recovers from those panics and returns an error in the usual way.
// This use of panic/recover is confined to the evaluator.

package api

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var defaultEnv = map[string]any{
	"true":     true,
	"false":    false,
	"nil":      nil,
	"contains": strings.Contains,
	"matches": func(target, re string) (bool, error) {
		return regexp.MatchString(re, target)
	},
	"hasPrefix": strings.HasPrefix,
	"hasSuffix": strings.HasSuffix,
}

// evaluate evaluates the given AST expression against the environment.
// It recovers from error panics and returns them as errors.
func evaluate(expr ast.Expr, env map[string]any) (val any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				panic(r)
			}
		}
	}()
	return eval(expr, env), nil
}

func eval(expr ast.Expr, env map[string]any) any {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return evalBasicLit(e)
	case *ast.Ident:
		return evalIdent(e, env)
	case *ast.CallExpr:
		return evalCall(e, env)
	case *ast.UnaryExpr:
		return evalUnary(e, env)
	case *ast.BinaryExpr:
		return evalBinary(e, env)
	case *ast.ParenExpr:
		return eval(e.X, env)
	default:
		failf("unsupported expression type: %T", expr)
		return nil
	}
}

func evalTo[T any](expr ast.Expr, env map[string]any) T {
	var zero T
	val := eval(expr, env)
	typedVal, ok := val.(T)
	if !ok {
		failf("expected type %T, got %T", zero, val)
	}
	return typedVal
}

func evalBasicLit(e *ast.BasicLit) any {
	switch e.Kind {
	case token.INT:
		v, err := strconv.Atoi(e.Value)
		if err != nil {
			fail(err)
		}
		return v
	case token.STRING:
		v, err := strconv.Unquote(e.Value)
		if err != nil {
			fail(err)
		}
		return v
	default:
		failf("unsupported basic lit kind: %v", e.Kind)
		return nil
	}
}

func evalIdent(e *ast.Ident, env map[string]any) any {
	if val, ok := defaultEnv[e.Name]; ok {
		return val
	}
	val, ok := env[e.Name]
	if !ok {
		failf("undefined identifier: %s", e.Name)
	}
	return val
}

func evalUnary(e *ast.UnaryExpr, env map[string]any) any {
	switch e.Op {
	case token.SUB: // unary -
		return -evalTo[int](e.X, env)
	case token.NOT: // !
		return !evalTo[bool](e.X, env)
	default:
		failf("unsupported unary operator: %v", e.Op)
		return nil
	}
}

func evalBinary(e *ast.BinaryExpr, env map[string]any) any {
	switch e.Op {
	case token.ADD: // +
		leftVal := eval(e.X, env)
		switch l := leftVal.(type) {
		case int:
			r := evalTo[int](e.Y, env)
			return l + r
		case string:
			r := evalTo[string](e.Y, env)
			return l + r
		default:
			failf("invalid type for +: %T", leftVal)
			return nil
		}
	case token.SUB: // -
		return evalTo[int](e.X, env) - evalTo[int](e.Y, env)
	case token.MUL: // *
		return evalTo[int](e.X, env) * evalTo[int](e.Y, env)
	case token.QUO: // /
		r := evalTo[int](e.Y, env)
		if r == 0 {
			failf("division by zero")
		}
		return evalTo[int](e.X, env) / r
	case token.REM: // %
		r := evalTo[int](e.Y, env)
		if r == 0 {
			failf("division by zero")
		}
		return evalTo[int](e.X, env) % r
	case token.LSS, token.GTR, token.LEQ, token.GEQ:
		var c int
		x := eval(e.X, env)
		switch x := x.(type) {
		case int:
			y := evalTo[int](e.Y, env)
			c = cmp.Compare(x, y)
		case string:
			y := evalTo[string](e.Y, env)
			c = cmp.Compare(x, y)
		default:
			failf("invalid type for %s: %T", e.Op, x)
			return nil
		}
		switch e.Op {
		case token.LSS:
			return c < 0
		case token.GTR:
			return c > 0
		case token.LEQ:
			return c <= 0
		case token.GEQ:
			return c >= 0
		default:
			panic("bug: missing case")
		}

	case token.EQL: // ==
		return eval(e.X, env) == eval(e.Y, env)
	case token.NEQ: // !=
		return eval(e.X, env) != eval(e.Y, env)
	case token.LAND:
		return evalTo[bool](e.X, env) && evalTo[bool](e.Y, env)
	case token.LOR:
		return evalTo[bool](e.X, env) || evalTo[bool](e.Y, env)
	default:
		failf("unsupported binary operator: %v", e.Op)
		return nil
	}
}

func evalCall(e *ast.CallExpr, env map[string]any) any {
	fnVal := eval(e.Fun, env)

	v := reflect.ValueOf(fnVal)
	if v.Kind() != reflect.Func {
		failf("not a function: %v (type %T)", fnVal, fnVal)
	}

	t := v.Type()
	if t.NumOut() > 2 {
		failf("function has too many return values: %d", t.NumOut())
	}

	// Check argument count first
	if t.IsVariadic() {
		failf("variadic functions are not supported")
	}
	if len(e.Args) != t.NumIn() {
		failf("argument count mismatch: got %d, expected %d", len(e.Args), t.NumIn())
	}

	args := make([]reflect.Value, len(e.Args))
	for i, argExpr := range e.Args {
		val := eval(argExpr, env)
		if val == nil {
			args[i] = reflect.Zero(t.In(i))
		} else {
			args[i] = reflect.ValueOf(val)
		}
	}

	var results []reflect.Value
	tryCall(func() {
		results = v.Call(args)
	})

	if len(results) == 0 {
		return nil
	}
	if len(results) == 1 {
		return results[0].Interface()
	}
	if len(results) == 2 {
		errVal := results[1].Interface()
		if errVal != nil {
			if err, ok := errVal.(error); ok {
				fail(err)
			}
			failf("second return value must be error, got %T", errVal)
		}
		return results[0].Interface()
	}
	failf("unexpected number of return values: %d", len(results))
	return nil
}

func tryCall(f func()) {
	defer func() {
		if r := recover(); r != nil {
			failf("panic during function call: %v", r)
		}
	}()
	f()
}

type fieldMap = map[string]reflect.StructField

var jsonFieldsMap sync.Map // from reflect.Type to fieldMap

// jsonFields collects all the fields in the struct t that marshal
// to JSON. It implements an approximation to the JSON rules
// as described in [json.Marshal]: considering only the visible
// fields of the struct as returned by [reflect.VisibleFields].
//
// jsonFields must not be called on recursive types.
func jsonFields(t reflect.Type) fieldMap {
	// Lock not necessary: at worst we'll duplicate work.
	if val, ok := jsonFieldsMap.Load(t); ok {
		return val.(fieldMap)
	}
	m := fieldMap{}
	for _, field := range reflect.VisibleFields(t) {
		// Skip anonymous fields.
		jname := jsonName(field)
		if jname != "" {
			if field.Anonymous {
				panic(fmt.Sprintf("anonymous field %s with json tag", field.Name))
			}
			m[jname] = field
		}
	}
	jsonFieldsMap.Store(t, m)
	return m
}

// jsonName returns the name that the field would be given
// by json.Marshal, or "" if none (unexported or omitted).
func jsonName(f reflect.StructField) string {
	if !f.IsExported() {
		return ""
	}
	tag, ok := f.Tag.Lookup("json")
	if !ok { // if no tag, use the field name
		return f.Name
	}
	name, _, found := strings.Cut(tag, ",")
	// "-" means omit, but "-," means the name is "-"
	if name == "-" && !found {
		return ""
	}
	return name
}
func fail(err error) {
	panic(err)
}

func failf(format string, a ...any) {
	panic(fmt.Errorf(format, a...))
}
