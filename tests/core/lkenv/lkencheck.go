// -*- Mode: Go; indent-tabs-mode: t -*-

// +build cgo

/*
 * Copyright (C) 2019 Canonical Ltd
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

// #cgo CFLAGS: -I${SRCDIR}/../../../include
// #include <lk/snappy_boot_v2.h>
import "C"

import (
	"fmt"
)

func main() {
	var run C.SNAP_RUN_BOOT_SELECTION_t
	fmt.Println(run.crc32)
}
