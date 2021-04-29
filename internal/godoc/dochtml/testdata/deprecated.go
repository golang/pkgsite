// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package deprecated has some deprecated symbols.
package deprecated

const GoodC = 1

// BadC is bad.
// Deprecated: use GoodC.
const BadC = 2

var GoodV = 1

// Deprecated: use GoodV.
var BadV = 2

func GoodF() {}

/*
   BadF is bad.
   Deprecated: use GoodF.
*/
func BadF() {}

type GoodT int

func NewGoodTGood() GoodT {}

// NewGoodTBad is bad.
// Deprecated: use NewGoodTGood.
func NewGoodTBad() GoodT {}

func (GoodT) GoodM() {}

// BadM is bad.
// Deprecated: use GoodM.
func (GoodT) BadM() {}

// BadT is bad.
// Deprecated: use GoodT.
// Don't use this.
type BadT int

func NewBadTGood() BadT {}

// Deprecated: use NewBadTGood.
func NewBadTBad() BadT {}

func (BadT) GoodM() {}

// Deprecated: use GoodM.
func (BadT) BadM() {}
