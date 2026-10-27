// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2025 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jessevdk/go-flags"

	"github.com/snapcore/snapd/desktop/iconbadge"
)

var opts struct {
	Key    string `long:"key" description:"Instance key to use for badging (required)"`
	Format string `long:"format" description:"Override format detection (png or svg)" choice:"png" choice:"svg"`
}

// normalizeArgs maps single-dash long options (-key) to double-dash (--key)
// for go-flags.
func normalizeArgs(args []string) []string {
	known := map[string]bool{"key": true, "format": true}
	out := make([]string, 0, len(args))
	for _, a := range args {
		if len(a) > 2 && a[0] == '-' && a[1] != '-' {
			name := a[1:]
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name = name[:eq]
			}
			if known[name] {
				out = append(out, "-"+a)
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

func main() {
	parser := flags.NewParser(&opts, flags.Default)
	// normalizeArgs works on os.Args[1:] (without program name) so that
	// positional arguments are returned cleanly by ParseArgs.
	args, err := parser.ParseArgs(normalizeArgs(os.Args[1:]))
	if err != nil {
		os.Exit(1)
	}

	if opts.Key == "" {
		fmt.Fprintln(os.Stderr, "error: --key is required")
		os.Exit(1)
	}

	var input []byte
	if len(args) >= 1 {
		input, err = os.ReadFile(args[0])
	} else {
		input, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read input: %v\n", err)
		os.Exit(1)
	}

	ext := opts.Format
	if ext == "" {
		if len(args) >= 1 {
			ext = filepath.Ext(args[0])
		} else {
			fmt.Fprintln(os.Stderr, "cannot determine format from stdin without --format")
			os.Exit(1)
		}
	}

	out, err := iconbadge.Badge(input, ext, opts.Key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot badge icon: %v\n", err)
		os.Exit(1)
	}

	if len(args) >= 2 {
		err = os.WriteFile(args[1], out, 0644)
	} else {
		_, err = os.Stdout.Write(out)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot write output: %v\n", err)
		os.Exit(1)
	}
}
