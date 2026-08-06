// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package tri defines a 3-valued bool-like value,
// [tri.Bool].
package tri

import (
	"encoding/json"
	"errors"
	"fmt"
)

// A Bool is a three-valued boolean value.
type Bool int

const (
	No Bool = iota
	Maybe
	Yes
)

func (t Bool) String() string {
	switch t {
	case Yes:
		return "Yes"
	case No:
		return "No"
	case Maybe:
		return "Maybe"
	default:
		return fmt.Sprintf("tri.Bool(%d)", t)
	}
}

// FromBool returns a tri.Bool from a bool.
func FromBool(b bool) Bool {
	if b {
		return Yes
	}
	return No
}

// And computes the "and" operation on two [Bool]s.
// If the Bools are both Yes or No, then And behaves
// normally. If either is Maybe, then the result is
// No if the other is No, otherwise Maybe.
func (t1 Bool) And(t2 Bool) Bool {
	switch t1 {
	case Yes:
		return t2
	case No:
		return No
	case Maybe:
		if t2 == No {
			return No
		}
		return Maybe
	default:
		panic(fmt.Sprintf("bad tri.Bool: %+v", t1))
	}
}

// Or computes the "or" operation on two [Bool]s.
// If the Bools are both Yes or No, then Or behaves
// normally. If either is Maybe, then the result is
// Yes if the other is Yes, otherwise Maybe.
func (t1 Bool) Or(t2 Bool) Bool {
	switch t1 {
	case Yes:
		return Yes
	case No:
		return t2
	case Maybe:
		if t2 == Yes {
			return Yes
		}
		return Maybe
	default:
		panic(fmt.Sprintf("bad tri.Bool: %+v", t1))
	}
}

// Not negates t. If t is Yes or No, Not behaves
// normally. If t is Maybe, Not returns Maybe.
func (t Bool) Not() Bool {
	switch t {
	case Yes:
		return No
	case No:
		return Yes
	case Maybe:
		return Maybe
	default:
		panic(fmt.Sprintf("bad tri.Bool: %+v", t))
	}
}

var (
	yesBytes   = []byte(`"Y"`)
	noBytes    = []byte(`"N"`)
	maybeBytes = []byte(`"?"`)
)

func (t Bool) MarshalJSON() ([]byte, error) {
	switch t {
	case Yes:
		return yesBytes, nil
	case No:
		return noBytes, nil
	case Maybe:
		return maybeBytes, nil
	default:
		return nil, errors.New("bad tri.Bool")
	}
}

func (t *Bool) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "Y":
		*t = Yes
	case "N":
		*t = No
	case "?":
		*t = Maybe
	default:
		return fmt.Errorf("tri.Bool.UnmarshalJSON: bad tri.Bool: %q", s)
	}
	return nil
}
