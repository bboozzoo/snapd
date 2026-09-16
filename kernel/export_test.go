// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2024 Canonical Ltd
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

package kernel

// MockEnsureInterval sets the overlord ensure interval for tests.
func MockOsSymlink(newSymlink func(string, string) error) (restore func()) {
	old := osSymlink
	osSymlink = newSymlink
	return func() { osSymlink = old }
}

// WriteDriversTreeMeta is exported for testing.
func WriteDriversTreeMeta(destDir string) error {
	return writeDriversTreeMeta(destDir)
}

// ReadDriversTreeGeneratorVersion is exported for testing.
func ReadDriversTreeGeneratorVersion(destDir string) (int, error) {
	return readDriversTreeGeneratorVersion(destDir)
}

// KernelDriversTreeGeneratorVersion returns the current generator version
// constant, exported for testing.
func KernelDriversTreeGeneratorVersion() int {
	return kernelDriversTreeGeneratorVersion
}

// MockKernelDriversTreeGeneratorVersion overrides the generator version
// constant for testing (e.g. to simulate a revert scenario where the
// on-disk marker records a newer version than the running code).
func MockKernelDriversTreeGeneratorVersion(v int) (restore func()) {
	old := kernelDriversTreeGeneratorVersion
	kernelDriversTreeGeneratorVersion = v
	return func() { kernelDriversTreeGeneratorVersion = old }
}
