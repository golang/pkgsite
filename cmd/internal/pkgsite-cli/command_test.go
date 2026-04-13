// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsageLine(t *testing.T) {
	binName := filepath.Base(os.Args[0])
	tests := []struct {
		name string
		c    *command
		want string
	}{
		{
			name: "no args",
			c:    &command{name: "version"},
			want: binName + " version",
		},
		{
			name: "with args",
			c:    &command{name: "package", args: "<package>[@version]"},
			want: binName + " package <package>[@version] [flags]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.usageLine(); got != tt.want {
				t.Errorf("usageLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDispatch(t *testing.T) {
	dummyCmd := &command{
		name:    "dummy",
		summary: "dummy command",
		run: func(fs *flag.FlagSet, stdout, stderr io.Writer) int {
			return 0
		},
	}

	flagsCmd := &command{
		name:    "flags",
		summary: "command with flags",
		flags:   flag.NewFlagSet("flags", flag.ContinueOnError),
		run: func(fs *flag.FlagSet, stdout, stderr io.Writer) int {
			return 0
		},
	}

	cmds := []*command{dummyCmd, flagsCmd}

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "no arguments",
			args:       []string{},
			wantExit:   2,
			wantStderr: "Usage:",
		},
		{
			name:       "only flags",
			args:       []string{"-json"},
			wantExit:   2,
			wantStderr: "Usage:",
		},
		{
			name:       "known command",
			args:       []string{"dummy"},
			wantExit:   0,
			wantStdout: "",
		},
		{
			name:       "known command with flags",
			args:       []string{"flags"},
			wantExit:   0,
			wantStdout: "",
		},
		{
			name:       "unknown command",
			args:       []string{"unknown"},
			wantExit:   2,
			wantStderr: "unknown command: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exit := dispatch(tt.args, cmds, &stdout, &stderr)
			if exit != tt.wantExit {
				t.Errorf("dispatch() exit = %d, want %d", exit, tt.wantExit)
			}
			if tt.wantStdout != "" && !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("dispatch() stdout = %q, want to contain %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("dispatch() stderr = %q, want to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestVersionInfo(t *testing.T) {
	v := versionInfo()
	binName := filepath.Base(os.Args[0])
	if !strings.HasPrefix(v, binName) {
		t.Errorf("versionInfo() = %q, want to start with %q", v, binName)
	}
}
