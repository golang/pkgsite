// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"

	"contrib.go.opencensus.io/integrations/ocsql"
)

// RegisterOCWrapper registers a driver that wraps the OpenCensus driver, which in
// turn wraps the driver named as the first argument.
func RegisterOCWrapper(driverName string, opts ...ocsql.TraceOption) (string, error) {
	// Get the driver to wrap.
	db, err := sql.Open(driverName, "")
	if err != nil {
		return "", err
	}
	dri := db.Driver()
	if err := db.Close(); err != nil {
		return "", err
	}
	name := "ocWrapper-" + driverName
	sql.Register(name, &wrapOCDriver{dri, opts})
	return name, nil
}

type wrapOCDriver struct {
	underlying driver.Driver
	opts       []ocsql.TraceOption
}

// Open implements database/sql/driver.Driver.
func (d *wrapOCDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.underlying.Open(name)
	if err != nil {
		return nil, err
	}
	oc := ocsql.WrapConn(conn, d.opts...)
	return &wrapConn{conn, oc.(iconn)}, nil
}

type iconn interface {
	driver.Pinger
	driver.ExecerContext
	driver.QueryerContext
	driver.Conn
	driver.ConnPrepareContext
	driver.ConnBeginTx
}

// A wrapConn knows about both the underlying Conn, and the OpenCensus Conn that wraps it.
// It delegates all calls to the OpenCensus Conn, but the underlying conn is available
// to this package.
type wrapConn struct {
	underlying driver.Conn
	oc         iconn
}

// Ping and all the following methods implement driver.Conn and related interfaces,
// listed in the iconn interface above.
func (c *wrapConn) Ping(ctx context.Context) error { return c.oc.Ping(ctx) }

func (c *wrapConn) Prepare(query string) (driver.Stmt, error) { return c.oc.Prepare(query) }
func (c *wrapConn) Close() error                              { return c.oc.Close() }
func (c *wrapConn) Begin() (driver.Tx, error)                 { return nil, errors.New("unimplmented") }

func (c *wrapConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	return c.oc.ExecContext(ctx, q, args)
}

func (c *wrapConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	return c.oc.QueryContext(ctx, q, args)
}

func (c *wrapConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.oc.PrepareContext(ctx, query)
}

func (c *wrapConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.oc.BeginTx(ctx, opts)
}
