// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postgres

import (
	"testing"
	"time"
)

const testTimeout = 5 * time.Second

var testDB *DB

func TestMain(m *testing.M) {
	RunDBTests("discovery_postgres_test", m, &testDB)
}
