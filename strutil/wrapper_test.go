// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016 Canonical Ltd
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

package strutil_test

import (
	"bytes"

	"gopkg.in/check.v1"

	"github.com/snapcore/snapd/strutil"
)

type wrapperSuite struct{}

var _ = check.Suite(&wrapperSuite{})

func (wrapperSuite) TestWrap(c *check.C) {
	for _, tc := range []struct {
		in     string
		indent string
		out    string
	}{{
		in:     "",
		indent: "  ",
		out:    "  ",
	}, {
		in:  "   one:",
		out: "   one:",
	}, {
		in:  "   one:\n",
		out: "   one:\n",
	}, {
		in: `one:
 * two three 日本語 four five six
   * seven height nine ten
`,
		indent: "  ",
		out: `  one:
   * two three 日本語
   four five six
     * seven height
     nine ten
`,
	}, {
		in: "abcdefghijklm nopqrstuvwxyz ABCDEFGHIJKLMNOPQR STUVWXYZ",
		out: `
  abcdefghijklm
  nopqrstuvwxyz
  ABCDEFGHIJKLMNOPQR
  STUVWXYZ
`[1:],
	}} {
		var buf bytes.Buffer
		w := strutil.WordWrapper{
			Indent: tc.indent,
			Width:  20,
		}
		// io.Copy(buf, w)
		c.Logf("text: %q", tc.in)
		w.Wrap(&buf, bytes.NewBufferString(tc.in))
		c.Check(buf.String(), check.Equals, tc.out, check.Commentf("%q", tc.in))
	}
}
