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

package iconbadge

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"
)

// ErrUnbadgeable is returned when an icon cannot be badged.
var ErrUnbadgeable = errors.New("icon cannot be badged")

// Badge color saturation/lightness in HSL.
const (
	badgeSaturation = 0.65
	badgeLightness  = 0.5
)

// badgeDiameterFraction is the badge diameter as a fraction of min(W,H).
const badgeDiameterFraction = 0.30

// BadgeColor returns a deterministic opaque color derived from the instance
// key; the same key always produces the same color.
func BadgeColor(key string) color.RGBA {
	h := fnv.New32a()
	h.Write([]byte(key))
	hue := float64(h.Sum32() % 360)
	return hslToRGBA(hue, badgeSaturation, badgeLightness)
}

// Badge adds a parallel-instance badge to an icon. Only .png and .svg are
// supported; any other extension or decode error yields ErrUnbadgeable.
func Badge(src []byte, ext, instanceKey string) ([]byte, error) {
	e := strings.ToLower(ext)
	e = strings.TrimPrefix(e, ".")
	switch e {
	case "png":
		return badgePNG(src, instanceKey)
	case "svg":
		return badgeSVG(src, instanceKey)
	default:
		return nil, ErrUnbadgeable
	}
}

// badgePNG composites a colored badge circle with a thin white ring at the
// bottom-right corner and re-encodes.
func badgePNG(src []byte, instanceKey string) ([]byte, error) {
	srcImg, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, ErrUnbadgeable
	}

	dst := image.NewNRGBA(srcImg.Bounds())
	draw.Draw(dst, dst.Bounds(), srcImg, srcImg.Bounds().Min, draw.Src)

	w := dst.Rect.Dx()
	h := dst.Rect.Dy()
	minDim := math.Min(float64(w), float64(h))
	diam := math.Round(badgeDiameterFraction * minDim)
	r := diam / 2
	if r < 0.5 {
		return nil, ErrUnbadgeable
	}
	margin := r * 0.35
	cx := float64(w) - r - margin
	cy := float64(h) - r - margin

	badgeColor := BadgeColor(instanceKey)

	// White ring for contrast at small sizes.
	ringR := r + 1
	ringMask := circleAlphaMask(dst.Bounds(), cx, cy, ringR)
	draw.DrawMask(dst, dst.Bounds(), &image.Uniform{color.RGBA{R: 255, G: 255, B: 255, A: 255}}, image.Point{}, ringMask, image.Point{}, draw.Over)

	fillMask := circleAlphaMask(dst.Bounds(), cx, cy, r)
	draw.DrawMask(dst, dst.Bounds(), &image.Uniform{badgeColor}, image.Point{}, fillMask, image.Point{}, draw.Over)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, ErrUnbadgeable
	}
	return buf.Bytes(), nil
}

// circleAlphaMask returns an anti-aliased circle mask with coverage
// alpha = clamp(0, 255, 255*(r+0.5-dist)).
func circleAlphaMask(bounds image.Rectangle, cx, cy, r float64) *image.Alpha {
	mask := image.NewAlpha(bounds)
	rPlus := r + 0.5
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		dy := float64(y) - cy
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dx := float64(x) - cx
			dist := math.Sqrt(dx*dx + dy*dy)
			cov := 255 * (rPlus - dist)
			if cov < 0 {
				cov = 0
			} else if cov > 255 {
				cov = 255
			}
			mask.SetAlpha(x, y, color.Alpha{A: uint8(cov)})
		}
	}
	return mask
}

// badgeSVG splices a badge <g> (colored circle + thin white ring) before the
// closing </svg>. Dimensions come from width/height, falling back to viewBox.
func badgeSVG(src []byte, instanceKey string) ([]byte, error) {
	svgStr := string(src)
	closeIdx := lastIndexCaseInsensitive(svgStr, "</svg>")
	if closeIdx < 0 {
		return nil, ErrUnbadgeable
	}

	w, h, ok := resolveSVGDimensions(svgStr)
	if !ok {
		return nil, ErrUnbadgeable
	}

	minDim := math.Min(w, h)
	diam := math.Round(badgeDiameterFraction * minDim)
	r := diam / 2
	if r < 0.5 {
		return nil, ErrUnbadgeable
	}
	margin := r * 0.35
	cx := w - r - margin
	cy := h - r - margin

	badgeColor := BadgeColor(instanceKey)
	fillHex := fmt.Sprintf("#%02x%02x%02x", badgeColor.R, badgeColor.G, badgeColor.B)
	ringR := r + 1

	insertion := fmt.Sprintf(
		`<g id="snapd-parallel-instance-badge"><circle cx="%s" cy="%s" r="%s" fill="#ffffff"/><circle cx="%s" cy="%s" r="%s" fill="%s"/></g>`,
		formatCoord(cx), formatCoord(cy), formatCoord(ringR),
		formatCoord(cx), formatCoord(cy), formatCoord(r),
		fillHex,
	)

	out := make([]byte, 0, len(src)+len(insertion))
	out = append(out, src[:closeIdx]...)
	out = append(out, []byte(insertion)...)
	out = append(out, src[closeIdx:]...)
	return out, nil
}

// formatCoord formats a float without trailing zeros or exponent.
func formatCoord(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// resolveSVGDimensions returns the root <svg> viewport from width/height,
// falling back to viewBox ("minx miny w h").
func resolveSVGDimensions(svg string) (w, h float64, ok bool) {
	w, h, ok = parseWidthHeight(svg)
	if ok {
		return w, h, true
	}
	return parseViewBox(svg)
}

// parseWidthHeight parses the root <svg> width/height; ok=false if either is
// missing or uses a relative unit (e.g. %).
func parseWidthHeight(svg string) (w, h float64, ok bool) {
	wVal := attrValue(svg, "width")
	hVal := attrValue(svg, "height")
	if wVal == "" || hVal == "" {
		return 0, 0, false
	}
	w, wok := parseLength(wVal)
	h, hok := parseLength(hVal)
	if !wok || !hok {
		return 0, 0, false
	}
	return w, h, true
}

// parseViewBox parses viewBox as "minx miny w h".
func parseViewBox(svg string) (w, h float64, ok bool) {
	vb := attrValue(svg, "viewBox")
	if vb == "" {
		return 0, 0, false
	}
	fields := strings.Fields(vb)
	if len(fields) != 4 {
		return 0, 0, false
	}
	w, errW := strconv.ParseFloat(fields[2], 64)
	h, errH := strconv.ParseFloat(fields[3], 64)
	if errW != nil || errH != nil {
		return 0, 0, false
	}
	return w, h, true
}

// parseLength parses an SVG length, stripping a trailing "px"; relative
// units (e.g. %) are rejected.
func parseLength(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.HasSuffix(s, "%") {
		return 0, false
	}
	s = strings.TrimSuffix(s, "px")
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// attrValue extracts the value of the named attribute from the root <svg> tag.
func attrValue(svg, name string) string {
	svgIdx := indexCaseInsensitive(svg, "<svg")
	if svgIdx < 0 {
		return ""
	}
	tagEnd := strings.IndexByte(svg[svgIdx:], '>')
	if tagEnd < 0 {
		return ""
	}
	tag := svg[svgIdx : svgIdx+tagEnd]

	search := name + "="
	idx := indexCaseInsensitive(tag, search)
	if idx < 0 {
		return ""
	}
	start := idx + len(search)
	if start >= len(tag) {
		return ""
	}
	quote := tag[start]
	if quote != '"' && quote != '\'' {
		return ""
	}
	end := strings.IndexByte(tag[start+1:], quote)
	if end < 0 {
		return ""
	}
	return tag[start+1 : start+1+end]
}

// indexCaseInsensitive returns the first case-insensitive index of substr, or -1.
func indexCaseInsensitive(s, substr string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(substr))
}

// lastIndexCaseInsensitive returns the last case-insensitive index of substr, or -1.
func lastIndexCaseInsensitive(s, substr string) int {
	return strings.LastIndex(strings.ToLower(s), strings.ToLower(substr))
}

// hslToRGBA converts HSL (h in degrees, s/l in [0,1]) to an opaque color.RGBA.
func hslToRGBA(h, s, l float64) color.RGBA {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return color.RGBA{
		R: uint8(math.Round((r + m) * 255)),
		G: uint8(math.Round((g + m) * 255)),
		B: uint8(math.Round((b + m) * 255)),
		A: 255,
	}
}
