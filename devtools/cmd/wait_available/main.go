// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/pkgsite/internal/log"
)

var timeout = flag.Duration("timeout", 15*time.Second, "timeout duration")

func main() {
	flag.Parse()
	ctx := context.Background()

	args := flag.Args()
	if len(args) < 1 {
		log.Fatalf(ctx, "expected at least one argument; got none")
	}
	hostport := args[0]
	var command []string

	if len(args) > 1 {
		if args[1] != "--" {
			log.Fatalf(ctx, "expected second argument to be \"--\"; got %q", args[1])
		}
		command = args[2:]
	}

	start := time.Now()
	for {
		conn, err := net.DialTimeout("tcp", hostport, 1*time.Second)
		if err == nil {
			conn.Close()
			break
		}
		if time.Since(start) > *timeout {
			log.Fatalf(ctx, "timed out after %v waiting for %s: %v", *timeout, hostport, err)
		}
		time.Sleep(1 * time.Second)
	}
	if len(command) == 0 {
		return
	}
	var err error
	binpath := command[0]
	if !filepath.IsAbs(binpath) {
		binpath, err = exec.LookPath(command[0])
		if err != nil {
			log.Fatalf(ctx, "looking up err: %v", err)
		}
	}
	if err := syscall.Exec(binpath, command, os.Environ()); err != nil {
		log.Fatalf(ctx, "exec-ing binary: %v", err)
	}
}
