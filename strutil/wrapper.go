// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2018 Canonical Ltd
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

package strutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"unicode"
)

type WordWrapper struct {
	Indent string
	Width  int
}

// func (w *WordWrapper) Read(b []byte) (int, error) {

// }

type wrapperState int

const (
	readingLeadingSpace wrapperState = iota
	readingWord
	readingSpace
)

func (state wrapperState) String() string {
	switch state {
	case readingLeadingSpace:
		return "<reading-leading-space>"
	case readingSpace:
		return "<reading-space>"
	case readingWord:
		return "<reading-word>"
	}
	return "<unknown>"
}

func (w *WordWrapper) Wrap(dst io.Writer, src io.Reader) error {

	in := bufio.NewReader(src)
	rout := bufio.NewWriter(dst)
	word := bytes.Buffer{}

	dentBuf := bytes.NewBufferString(w.Indent)
	dent := ""
	perLine := w.Width - len(dent)
	thisLine := perLine
	wordLen := 0

	state := readingLeadingSpace

reader:
	for {
		r, _, err := in.ReadRune()
		fmt.Printf("(%v) got rune: %v '%c' err: %v, this line: %v per line: %v word len: %v\n",
			state, r, r, err, thisLine, perLine, wordLen)
		// if err == io.EOF {
		// 	// rout.WriteString(dent)
		// 	if state == readingWord {
		// 		rout.Write(word.Bytes())
		// 		rout.WriteRune('\n')
		// 	}
		// 	break
		// }
		if err != nil && err != io.EOF {
			return err
		}
		switch state {
		case readingLeadingSpace:
			if err == io.EOF {
				dent = dentBuf.String()
				rout.WriteString(dent)
				break reader
			}

			if !unicode.IsSpace(r) {
				dent = dentBuf.String()
				perLine = w.Width - len(dent)
				thisLine = perLine

				state = readingWord

				fmt.Printf("<< unread\n")
				in.UnreadRune()
				rout.WriteString(dent)
			} else {
				dentBuf.WriteRune(' ')
			}
		case readingWord:
			if unicode.IsSpace(r) || err == io.EOF {
				if word.Len() > 0 {
					if wordLen > thisLine {
						fmt.Printf("-- newline\n")
						rout.WriteRune('\n')
						rout.WriteString(dent)
						thisLine = perLine
					}
					rout.Write(word.Bytes())
					word.Reset()
					thisLine -= wordLen
					wordLen = 0
				}

				if err == io.EOF {
					break reader
				}
				state = readingSpace
				fmt.Printf("<< unread\n")
				in.UnreadRune()
				continue
			}
			word.WriteRune(r)
			wordLen += 1
			// thisLine -= 1
		case readingSpace:
			if err == io.EOF {
				break reader
			}

			if thisLine <= 0 || r == '\n' {
				fmt.Printf("-- newline\n")
				rout.WriteRune('\n')
				thisLine = perLine
				if r == '\n' {
					state = readingLeadingSpace
					dentBuf.Reset()
					dentBuf.WriteString(w.Indent)
				}
			}
			if !unicode.IsSpace(r) {
				thisLine -= 1
				state = readingWord
				fmt.Printf("<< unread\n")
				in.UnreadRune()
			}
		}
	}
	rout.Flush()
	// for len([]rune(text)) > width {
	// 	idx = len(text)
	// 	for idx > 0 && len([]rune(text[:idx])) > width {
	// 		idx = strings.LastIndexFunc(text[:idx], unicode.IsSpace)
	// 	}
	// 	if idx < 0 {
	// 		idx = width
	// 	}
	// 	output(text[:idx])
	// 	text = text[idx:]

	// 	for idx, c = range text {
	// 		if !unicode.IsSpace(c) {
	// 			break
	// 		}
	// 		idx++
	// 	}
	// 	text = text[idx:]
	// }

	// output(text)
	return nil
}
