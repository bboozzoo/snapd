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

package iconbadge_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/desktop/iconbadge"
)

func TestIconbadge(t *testing.T) { TestingT(t) }

type iconbadgeSuite struct{}

var _ = Suite(&iconbadgeSuite{})

func (s *iconbadgeSuite) TestBadgeColorDeterministic(c *C) {
	c1 := iconbadge.BadgeColor("key-one")
	c2 := iconbadge.BadgeColor("key-one")
	c.Check(c1, Equals, c2)
	// Different keys should generally differ; just check it's not identical
	// for a clearly different key.
	c3 := iconbadge.BadgeColor("key-two")
	if c1 == c3 {
		c.Fatalf("expected different colors for different keys, got %v for both", c1)
	}
	// Color must be fully opaque.
	c.Check(c1.A, Equals, uint8(255))
}

func (s *iconbadgeSuite) TestBadgePNG(c *C) {
	// Synthesize a 64×64 solid blue PNG.
	srcImg := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			srcImg.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var srcBuf bytes.Buffer
	c.Assert(png.Encode(&srcBuf, srcImg), IsNil)
	srcBytes := srcBuf.Bytes()

	out, err := iconbadge.Badge(srcBytes, ".png", "k1")
	c.Assert(err, IsNil)

	outImg, err := png.Decode(bytes.NewReader(out))
	c.Assert(err, IsNil)
	c.Check(outImg.Bounds().Dx(), Equals, 64)
	c.Check(outImg.Bounds().Dy(), Equals, 64)

	// Badge center is bottom-right. diam = round(0.30*64) = 19; r = 9.5;
	// margin = 3.325; cx = cy = 64 - 9.5 - 3.325 = 51.175.
	badgeColor := iconbadge.BadgeColor("k1")
	px := outImg.At(51, 51)
	r, g, b, _ := px.RGBA()
	c.Check(uint8(r>>8), Equals, badgeColor.R)
	c.Check(uint8(g>>8), Equals, badgeColor.G)
	c.Check(uint8(b>>8), Equals, badgeColor.B)
}

func (s *iconbadgeSuite) TestBadgeSVG(c *C) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="blue"/></svg>`
	out, err := iconbadge.Badge([]byte(svg), ".svg", "k1")
	c.Assert(err, IsNil)
	outStr := string(out)

	// Must contain the badge group.
	c.Check(strings.Contains(outStr, `id="snapd-parallel-instance-badge"`), Equals, true)
	// Must contain a circle element.
	c.Check(strings.Contains(outStr, "<circle"), Equals, true)
	// The original closing tag must still be present.
	c.Check(strings.Contains(outStr, "</svg>"), Equals, true)
	// The insertion must be before the closing </svg>.
	badgeIdx := strings.Index(outStr, `id="snapd-parallel-instance-badge"`)
	closeIdx := strings.Index(outStr, "</svg>")
	c.Check(badgeIdx < closeIdx, Equals, true)

	// Verify computed coords: diam = round(0.30*64) = 19; r = 9.5;
	// margin = 3.325; cx = cy = 64 - 9.5 - 3.325 = 51.175.
	badgeColor := iconbadge.BadgeColor("k1")
	fillHex := colorToHex(badgeColor)
	c.Check(strings.Contains(outStr, fillHex), Equals, true)
}

func (s *iconbadgeSuite) TestBadgeSVGViewBoxOnly(c *C) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><rect width="128" height="128" fill="red"/></svg>`
	out, err := iconbadge.Badge([]byte(svg), ".svg", "k1")
	c.Assert(err, IsNil)
	outStr := string(out)
	c.Check(strings.Contains(outStr, `id="snapd-parallel-instance-badge"`), Equals, true)
}

func (s *iconbadgeSuite) TestBadgeSVGUnbadgeable(c *C) {
	// No width/height/viewBox.
	svg1 := `<svg xmlns="http://www.w3.org/2000/svg"><rect fill="red"/></svg>`
	_, err := iconbadge.Badge([]byte(svg1), ".svg", "k1")
	c.Check(err, Equals, iconbadge.ErrUnbadgeable)

	// width="100%" (relative unit).
	svg2 := `<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="100%"><rect fill="red"/></svg>`
	_, err = iconbadge.Badge([]byte(svg2), ".svg", "k1")
	c.Check(err, Equals, iconbadge.ErrUnbadgeable)

	// No </svg> at all.
	svg3 := `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64">`
	_, err = iconbadge.Badge([]byte(svg3), ".svg", "k1")
	c.Check(err, Equals, iconbadge.ErrUnbadgeable)
}

func (s *iconbadgeSuite) TestBadgeIdempotentBytes(c *C) {
	// PNG
	srcImg := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			srcImg.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	var srcBuf bytes.Buffer
	c.Assert(png.Encode(&srcBuf, srcImg), IsNil)
	pngBytes := srcBuf.Bytes()

	out1, err := iconbadge.Badge(pngBytes, ".png", "k1")
	c.Assert(err, IsNil)
	out2, err := iconbadge.Badge(pngBytes, ".png", "k1")
	c.Assert(err, IsNil)
	c.Check(bytes.Equal(out1, out2), Equals, true)

	// SVG
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><rect width="64" height="64" fill="blue"/></svg>`
	svgOut1, err := iconbadge.Badge([]byte(svg), ".svg", "k1")
	c.Assert(err, IsNil)
	svgOut2, err := iconbadge.Badge([]byte(svg), ".svg", "k1")
	c.Assert(err, IsNil)
	c.Check(bytes.Equal(svgOut1, svgOut2), Equals, true)
}

func (s *iconbadgeSuite) TestBadgeUnbadgeableExt(c *C) {
	for _, ext := range []string{".gif", ".bmp", "garbage", ""} {
		_, err := iconbadge.Badge([]byte("whatever"), ext, "k1")
		c.Check(err, Equals, iconbadge.ErrUnbadgeable, Commentf("ext=%q", ext))
	}
}

func (s *iconbadgeSuite) TestBadgePNGDecodeError(c *C) {
	_, err := iconbadge.Badge([]byte("not a png"), ".png", "k1")
	c.Check(err, Equals, iconbadge.ErrUnbadgeable)
}

func colorToHex(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
