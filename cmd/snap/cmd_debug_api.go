// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2021 Canonical Ltd
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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/jessevdk/go-flags"
	"github.com/snapcore/snapd/logger"
)

type cmdDebugAPI struct {
	clientMixin

	Pretty bool `long:"pretty"`

	Positional struct {
		Method  string   `positional-arg-name:"<method>"`
		Query   string   `positional-arg-name:"<query>"`
		Headers []string `positional-arg-name:"<headers>"`
	} `positional-args:"yes" required:"yes"`
}

func init() {
	addDebugCommand("api",
		"Execute raw query to snapd API",
		"Execute a raw query to snapd API. Complex input can be read from stdin, while output is printed to stdout.",
		func() flags.Commander {
			return &cmdDebugAPI{}
		}, nil, nil)
}

func (x *cmdDebugAPI) Execute(args []string) error {
	method := x.Positional.Method
	switch method {
	case "GET", "POST":
	default:
		return fmt.Errorf("unsupported method %q", method)
	}

	u, err := url.Parse(x.Positional.Query)
	if err != nil {
		return err
	}

	var in io.Reader
	if method == "POST" {
		in = Stdin
	}

	hdrs := x.Positional.Headers
	reqHdrs := make(map[string]string, len(hdrs))
	for _, arg := range x.Positional.Headers {
		split := strings.SplitN(arg, "=", 2)
		reqHdrs[split[0]] = split[1]
	}
	logger.Debugf("url: %v", u.Path)
	logger.Debugf("query: %v", u.RawQuery)
	logger.Debugf("headers: %s", reqHdrs)

	rsp, err := x.client.DebugRaw(context.Background(), x.Positional.Method, u.Path, u.Query(), reqHdrs, in)
	if rsp != nil {
		defer rsp.Body.Close()
	}
	if err != nil {
		return err
	}

	if x.Pretty {
		var temp map[string]interface{}
		if err := json.NewDecoder(rsp.Body).Decode(&temp); err != nil {
			return err
		}
		enc := json.NewEncoder(Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(temp); err != nil {
			return err
		}
	} else {
		if _, err := io.Copy(Stdout, rsp.Body); err != nil {
			return err
		}
	}

	return nil
}
