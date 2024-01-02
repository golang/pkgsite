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
		if time.Since(start) > *timeout {
			break
		}
		if conn, err := net.DialTimeout("tcp", hostport, 1*time.Second); err != nil {
			time.Sleep(1 * time.Second)
			continue
		} else {
			conn.Close()
			break
		}
	}
	var err error
	binpath := command[0]
	if !filepath.IsAbs(binpath) {
		binpath, err = exec.LookPath(command[0])
		if err != nil {
			log.Fatalf(ctx, "looking up err: %v", err)
		}
	}
	if len(command) > 0 {
		err := syscall.Exec(binpath, command, os.Environ())
		if err != nil {
			log.Fatalf(ctx, "exec-ing binary: %v", err)
		}
	}
}
