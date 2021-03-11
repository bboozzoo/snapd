// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2020 Canonical Ltd
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

package lkenv

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

func fuzzVersion(data []byte, v Version) int {
	d, err := ioutil.TempDir("", "lkenv-fuzz")
	if err != nil {
		panic(fmt.Errorf("cannot create temp directory: %v", err))
	}
	defer func() {
		if err := os.RemoveAll(d); err != nil {
			panic(fmt.Errorf("cannot cleanup temp directory: %v", err))
		}
	}()

	envPath := filepath.Join(d, "env")
	if err := ioutil.WriteFile(envPath, data, 0666); err != nil {
		panic(fmt.Errorf("cannot write env file: %v", err))
	}
	env := NewEnv(envPath, "", v)
	if err := env.Load(); err != nil {
		return 0
	}
	return 1
}

func Fuzz(data []byte) int {
	var ret int
	ret = ret + fuzzVersion(data, V1)
	ret = ret + fuzzVersion(data, V2Run)
	ret = ret + fuzzVersion(data, V2Recovery)
	return ret
}
