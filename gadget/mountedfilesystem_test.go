// -*- Mode: Go; indent-tabs-mode: t -*-

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
package gadget_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "gopkg.in/check.v1"

	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/testutil"
)

type mountedfilesystemTestSuite struct {
	dir    string
	backup string
}

var _ = Suite(&mountedfilesystemTestSuite{})

func (s *mountedfilesystemTestSuite) SetUpTest(c *C) {
	s.dir = c.MkDir()
	s.backup = c.MkDir()
}

type gadgetData struct {
	name, target, content string
}

func makeGadgetData(c *C, where string, data []gadgetData) {
	for _, en := range data {
		if strings.HasSuffix(en.name, "/") {
			err := os.MkdirAll(filepath.Join(where, en.name), 0755)
			c.Check(en.content, HasLen, 0)
			c.Assert(err, IsNil)
			continue
		}
		makeSizedFile(c, filepath.Join(where, en.name), 0, []byte(en.content))
	}
}

func verifyWrittenGadgetData(c *C, where string, data []gadgetData) {
	for _, en := range data {
		target := filepath.Join(where, en.target)
		c.Check(target, testutil.FileContains, en.content)
	}
}

func makeExistingData(c *C, where string, data []gadgetData) {
	for _, en := range data {
		if strings.HasSuffix(en.target, "/") {
			err := os.MkdirAll(filepath.Join(where, en.target), 0755)
			c.Check(en.content, HasLen, 0)
			c.Assert(err, IsNil)
			continue
		}
		makeSizedFile(c, filepath.Join(where, en.target), 0, []byte(en.content))
	}
}

type contentType int

const (
	typeFile contentType = iota
	typeDir
)

func verifyDirContents(c *C, where string, expected map[string]contentType) {
	cleanWhere := filepath.Clean(where)

	got := make(map[string]contentType)
	filepath.Walk(where, func(name string, fi os.FileInfo, err error) error {
		if name == where {
			return nil
		}
		suffixName := name[len(cleanWhere)+1:]
		t := typeFile
		if fi.IsDir() {
			t = typeDir
		}
		got[suffixName] = t

		for prefix := filepath.Dir(name); prefix != where; prefix = filepath.Dir(prefix) {
			delete(got, prefix[len(cleanWhere)+1:])
		}

		return nil
	})

	c.Assert(got, DeepEquals, expected)
}

func (s *mountedfilesystemTestSuite) TestWriteFile(c *C) {
	makeSizedFile(c, filepath.Join(s.dir, "foo"), 0, []byte("foo foo foo"))

	outDir := c.MkDir()

	// foo -> /foo
	err := gadget.WriteFile(filepath.Join(s.dir, "foo"), filepath.Join(outDir, "foo"), nil)
	c.Assert(err, IsNil)
	c.Check(filepath.Join(outDir, "foo"), testutil.FileEquals, []byte("foo foo foo"))

	// foo -> bar/foo
	err = gadget.WriteFile(filepath.Join(s.dir, "foo"), filepath.Join(outDir, "bar/foo"), nil)
	c.Assert(err, IsNil)
	c.Check(filepath.Join(outDir, "bar/foo"), testutil.FileEquals, []byte("foo foo foo"))

	// overwrites
	makeSizedFile(c, filepath.Join(outDir, "overwrite"), 0, []byte("disappear"))
	err = gadget.WriteFile(filepath.Join(s.dir, "foo"), filepath.Join(outDir, "overwrite"), nil)
	c.Assert(err, IsNil)
	c.Check(filepath.Join(outDir, "overwrite"), testutil.FileEquals, []byte("foo foo foo"))

	// unless told to preserve
	keepName := filepath.Join(outDir, "keep")
	makeSizedFile(c, keepName, 0, []byte("can't touch this"))
	err = gadget.WriteFile(filepath.Join(s.dir, "foo"), filepath.Join(outDir, "keep"), []string{keepName})
	c.Assert(err, IsNil)
	c.Check(filepath.Join(outDir, "keep"), testutil.FileEquals, []byte("can't touch this"))

	err = gadget.WriteFile(filepath.Join(s.dir, "not-found"), filepath.Join(outDir, "foo"), nil)
	c.Assert(err, ErrorMatches, "cannot copy .*: unable to open .*/not-found: .* no such file or directory")
}

func (s *mountedfilesystemTestSuite) TestWriteDirectoryContents(c *C) {
	gd := []gadgetData{
		{"boot-assets/splash", "splash", "splash"},
		{"boot-assets/some-dir/data", "some-dir/data", "data"},
		{"boot-assets/some-dir/empty-file", "some-dir/empty-file", ""},
		{"boot-assets/nested-dir/nested", "/nested-dir/nested", "nested"},
		{"boot-assets/nested-dir/more-nested/more", "/nested-dir/more-nested/more", "more"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := c.MkDir()
	// boot-assets/ -> / (contents of boot assets under /)
	err := gadget.WriteDirectory(filepath.Join(s.dir, "boot-assets")+"/", outDir+"/", nil)
	c.Assert(err, IsNil)

	verifyWrittenGadgetData(c, outDir, gd)
}

func (s *mountedfilesystemTestSuite) TestWriteDirectoryWhole(c *C) {
	gd := []gadgetData{
		{"boot-assets/splash", "boot-assets/splash", "splash"},
		{"boot-assets/some-dir/data", "boot-assets/some-dir/data", "data"},
		{"boot-assets/some-dir/empty-file", "boot-assets/some-dir/empty-file", ""},
		{"boot-assets/nested-dir/nested", "boot-assets/nested-dir/nested", "nested"},
		{"boot-assets/nested-dir/more-nested/more", "boot-assets//nested-dir/more-nested/more", "more"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := c.MkDir()
	// boot-assets -> / (boot-assets and children under /)
	err := gadget.WriteDirectory(filepath.Join(s.dir, "boot-assets"), outDir+"/", nil)
	c.Assert(err, IsNil)

	verifyWrittenGadgetData(c, outDir, gd)
}

func (s *mountedfilesystemTestSuite) TestWriteNonDirectory(c *C) {
	gd := []gadgetData{
		{name: "foo", content: "nested"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := c.MkDir()

	err := gadget.WriteDirectory(filepath.Join(s.dir, "foo")+"/", outDir, nil)
	c.Assert(err, ErrorMatches, `cannot specify trailing / for a source which is not a directory`)

	err = gadget.WriteDirectory(filepath.Join(s.dir, "foo"), outDir, nil)
	c.Assert(err, ErrorMatches, `source is not a directory`)
}

func (s *mountedfilesystemTestSuite) TestMountedWriterHappy(c *C) {
	gd := []gadgetData{
		{"foo", "foo-dir/foo", "foo foo foo"},
		{"bar", "bar-name", "bar bar bar"},
		{"boot-assets/splash", "splash", "splash"},
		{"boot-assets/some-dir/data", "some-dir/data", "data"},
		{"boot-assets/some-dir/data", "data-copy", "data"},
		{"boot-assets/some-dir/empty-file", "some-dir/empty-file", ""},
		{"boot-assets/nested-dir/nested", "/nested-copy/nested", "nested"},
		{"boot-assets/nested-dir/more-nested/more", "/nested-copy/more-nested/more", "more"},
	}
	makeGadgetData(c, s.dir, gd)
	err := os.MkdirAll(filepath.Join(s.dir, "boot-assets/empty-dir"), 0755)
	c.Assert(err, IsNil)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					// single file in target directory
					Source: "foo",
					Target: "/foo-dir/",
				}, {
					// single file under different name
					Source: "bar",
					Target: "/bar-name",
				}, {
					// whole directory contents
					Source: "boot-assets/",
					Target: "/",
				}, {
					// single file from nested directory
					Source: "boot-assets/some-dir/data",
					Target: "/data-copy",
				}, {
					// contents of nested directory under new target directory
					Source: "boot-assets/nested-dir/",
					Target: "/nested-copy/",
				},
			},
		},
	}

	outDir := c.MkDir()

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Write(outDir, nil)
	c.Assert(err, IsNil)

	verifyWrittenGadgetData(c, outDir, gd)
	c.Assert(osutil.IsDirectory(filepath.Join(outDir, "empty-dir")), Equals, true)
}

func (s *mountedfilesystemTestSuite) TestMountedWriterNonDirectory(c *C) {
	gd := []gadgetData{
		{name: "foo", content: "nested"},
	}
	makeGadgetData(c, s.dir, gd)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					// contents of nested directory under new target directory
					Source: "foo/",
					Target: "/nested-copy/",
				},
			},
		},
	}

	outDir := c.MkDir()

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Write(outDir, nil)
	c.Assert(err, ErrorMatches, `cannot write filesystem content of source:foo/: cannot specify trailing / for a source which is not a directory`)
}

func (s *mountedfilesystemTestSuite) TestMountedWriterErrorMissingSource(c *C) {
	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					// single file in target directory
					Source: "foo",
					Target: "/foo-dir/",
				},
			},
		},
	}

	outDir := c.MkDir()

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Write(outDir, nil)
	c.Assert(err, ErrorMatches, "cannot write filesystem content of source:foo: .*unable to open.*: no such file or directory")
}

func (s *mountedfilesystemTestSuite) TestMountedWriterErrorBadDestination(c *C) {
	makeSizedFile(c, filepath.Join(s.dir, "foo"), 0, []byte("foo foo foo"))

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "vfat",
			Content: []gadget.VolumeContent{
				{
					// single file in target directory
					Source: "foo",
					Target: "/foo-dir/",
				},
			},
		},
	}

	outDir := c.MkDir()

	err := os.Chmod(outDir, 0000)
	c.Assert(err, IsNil)

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Write(outDir, nil)
	c.Assert(err, ErrorMatches, "cannot write filesystem content of source:foo: cannot create .*: mkdir .* permission denied")
}

func (s *mountedfilesystemTestSuite) TestMountedWriterConflictingDestinationDirectoryErrors(c *C) {
	makeGadgetData(c, s.dir, []gadgetData{
		{name: "foo", content: "foo foo foo"},
		{name: "foo-dir", content: "bar bar bar"},
	})

	psOverwritesDirectoryWithFile := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					// single file in target directory
					Source: "foo",
					Target: "/foo-dir/",
				}, {
					// conflicts with /foo-dir directory
					Source: "foo-dir",
					Target: "/",
				},
			},
		},
	}

	outDir := c.MkDir()

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, psOverwritesDirectoryWithFile)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	// can't overwrite a directory with a file
	err = rw.Write(outDir, nil)
	c.Assert(err, ErrorMatches, fmt.Sprintf("cannot write filesystem content of source:foo-dir: cannot copy .*: unable to create %s/foo-dir: .* is a directory", outDir))

}

func (s *mountedfilesystemTestSuite) TestMountedWriterConflictingDestinationFileOk(c *C) {
	makeGadgetData(c, s.dir, []gadgetData{
		{name: "foo", content: "foo foo foo"},
		{name: "bar", content: "bar bar bar"},
	})
	psOverwritesFile := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/",
				}, {
					// overwrites data from preceding entry
					Source: "foo",
					Target: "/bar",
				},
			},
		},
	}

	outDir := c.MkDir()

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, psOverwritesFile)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Write(outDir, nil)
	c.Assert(err, IsNil)

	c.Check(osutil.FileExists(filepath.Join(outDir, "foo")), Equals, false)
	// overwritten
	c.Check(filepath.Join(outDir, "bar"), testutil.FileEquals, "foo foo foo")
}

func (s *mountedfilesystemTestSuite) TestMountedWriterErrorNested(c *C) {
	makeGadgetData(c, s.dir, []gadgetData{
		{name: "foo/foo-dir", content: "data"},
		{name: "foo/bar/baz", content: "data"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					// single file in target directory
					Source: "/",
					Target: "/foo-dir/",
				},
			},
		},
	}

	outDir := c.MkDir()

	makeSizedFile(c, filepath.Join(outDir, "/foo-dir/foo/bar"), 0, nil)

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Write(outDir, nil)
	c.Assert(err, ErrorMatches, "cannot write filesystem content of source:/: .* not a directory")
}

func (s *mountedfilesystemTestSuite) TestMountedWriterPreserve(c *C) {
	// some data for the gadget
	gdWritten := []gadgetData{
		{"foo", "foo-dir/foo", "data"},
		{"bar", "bar-name", "data"},
		{"boot-assets/splash", "splash", "data"},
		{"boot-assets/some-dir/data", "some-dir/data", "data"},
		{"boot-assets/some-dir/empty-file", "some-dir/empty-file", "data"},
		{"boot-assets/nested-dir/more-nested/more", "/nested-copy/more-nested/more", "data"},
	}
	gdNotWritten := []gadgetData{
		{"foo", "/foo", "data"},
		{"boot-assets/some-dir/data", "data-copy", "data"},
		{"boot-assets/nested-dir/nested", "/nested-copy/nested", "data"},
	}
	makeGadgetData(c, s.dir, append(gdWritten, gdNotWritten...))

	// these exist in the root directory and are preserved
	preserve := []string{
		// mix entries with leading / and without
		"/foo",
		"/data-copy",
		"nested-copy/nested",
		"not-listed", // not present in 'gadget' contents
	}
	// these are preserved, but don't exist in the root, so data from gadget
	// will be written
	preserveButNotPresent := []string{
		"/bar-name",
		"some-dir/data",
	}
	outDir := filepath.Join(c.MkDir(), "out-dir")

	for _, en := range preserve {
		p := filepath.Join(outDir, en)
		makeSizedFile(c, p, 0, []byte("can't touch this"))
	}

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "foo",
					Target: "/foo-dir/",
				}, {
					// would overwrite /foo
					Source: "foo",
					Target: "/",
				}, {
					// preserved, but not present, will be
					// written
					Source: "bar",
					Target: "/bar-name",
				}, {
					// some-dir/data is preserved, but not
					// preset, hence will be written
					Source: "boot-assets/",
					Target: "/",
				}, {
					// would overwrite /data-copy
					Source: "boot-assets/some-dir/data",
					Target: "/data-copy",
				}, {
					// would overwrite /nested-copy/nested
					Source: "boot-assets/nested-dir/",
					Target: "/nested-copy/",
				},
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Write(outDir, append(preserve, preserveButNotPresent...))
	c.Assert(err, IsNil)

	// files that existed were preserved
	for _, en := range preserve {
		p := filepath.Join(outDir, en)
		c.Check(p, testutil.FileEquals, "can't touch this")
	}
	// everything else was written
	verifyWrittenGadgetData(c, outDir, gdWritten)
}

func (s *mountedfilesystemTestSuite) TestMountedWriterNonFilePreserveError(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{name: "foo", content: "data"},
	}
	makeGadgetData(c, s.dir, gd)

	preserve := []string{
		// this will be a directory
		"foo",
	}
	outDir := filepath.Join(c.MkDir(), "out-dir")
	// will conflict with preserve entry
	err := os.MkdirAll(filepath.Join(outDir, "foo"), 0755)
	c.Assert(err, IsNil)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "/",
					Target: "/",
				},
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Write(outDir, preserve)
	c.Assert(err, ErrorMatches, `cannot map preserve entries for destination ".*/out-dir": preserved entry "foo" is a directory`)
}

func (s *mountedfilesystemTestSuite) TestMountedWriterImplicitDir(c *C) {
	gd := []gadgetData{
		{"boot-assets/nested-dir/nested", "/nested-copy/nested-dir/nested", "nested"},
		{"boot-assets/nested-dir/more-nested/more", "/nested-copy/nested-dir/more-nested/more", "more"},
	}
	makeGadgetData(c, s.dir, gd)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					// contents of nested directory under new target directory
					Source: "boot-assets/nested-dir",
					Target: "/nested-copy/",
				},
			},
		},
	}

	outDir := c.MkDir()

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Write(outDir, nil)
	c.Assert(err, IsNil)

	verifyWrittenGadgetData(c, outDir, gd)
}

func (s *mountedfilesystemTestSuite) TestMountedWriterNoFs(c *C) {
	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size: 2048,
			// no filesystem
			Content: []gadget.VolumeContent{
				{
					// single file in target directory
					Source: "foo",
					Target: "/foo-dir/",
				},
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, ErrorMatches, "structure #0 has no filesystem")
	c.Assert(rw, IsNil)
}

func (s *mountedfilesystemTestSuite) TestMountedWriterTrivialValidation(c *C) {
	rw, err := gadget.NewMountedFilesystemWriter(s.dir, nil)
	c.Assert(err, ErrorMatches, `internal error: \*PositionedStructure.*`)
	c.Assert(rw, IsNil)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			// no filesystem
			Content: []gadget.VolumeContent{
				{
					Source: "",
					Target: "",
				},
			},
		},
	}

	rw, err = gadget.NewMountedFilesystemWriter("", ps)
	c.Assert(err, ErrorMatches, `internal error: gadget content directory cannot be unset`)
	c.Assert(rw, IsNil)

	rw, err = gadget.NewMountedFilesystemWriter(s.dir, ps)
	c.Assert(err, IsNil)

	err = rw.Write("", nil)
	c.Assert(err, ErrorMatches, "internal error: destination directory cannot be unset")

	d := c.MkDir()
	err = rw.Write(d, nil)
	c.Assert(err, ErrorMatches, "cannot write filesystem content .* source cannot be unset")

	ps.Content[0].Source = "/"
	err = rw.Write(d, nil)
	c.Assert(err, ErrorMatches, "cannot write filesystem content .* target cannot be unset")
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterBackupSimple(c *C) {
	// some data for the gadget
	gdWritten := []gadgetData{
		{"bar", "bar-name", "data"},
		{"foo", "foo", "data"},
		{"zed", "zed", "data"},
		{"same-data", "same", "same"},
		// not included in volume contents
		{"not-written", "not-written", "data"},
	}
	makeGadgetData(c, s.dir, gdWritten)

	outDir := filepath.Join(c.MkDir(), "out-dir")

	// these exist in the destination directory and will be backed up
	backedUp := []gadgetData{
		{target: "foo", content: "can't touch this"},
		{target: "nested/foo", content: "can't touch this"},
		// listed in preserve
		{target: "zed", content: "preserved"},
		// same content as the update
		{target: "same", content: "same"},
	}
	makeExistingData(c, outDir, backedUp)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/bar-name",
				}, {
					Source: "foo",
					Target: "/",
				}, {
					Source: "foo",
					Target: "/nested/",
				}, {
					Source: "zed",
					Target: "/",
				}, {
					Source: "same-data",
					Target: "/same",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition:  1,
				Preserve: []string{"/zed"},
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Backup()
	c.Assert(err, IsNil)

	// files that existed were backed up
	for _, en := range backedUp {
		backup := filepath.Join(s.backup, "struct-0", en.target+".backup")
		same := filepath.Join(s.backup, "struct-0", en.target+".same")
		switch en.content {
		case "preserved":
			c.Check(osutil.FileExists(backup), Equals, false, Commentf("file: %v", backup))
			c.Check(osutil.FileExists(same), Equals, false, Commentf("file: %v", same))
		case "same":
			c.Check(osutil.FileExists(same), Equals, true, Commentf("file: %v", same))
		default:
			c.Check(backup, testutil.FileEquals, "can't touch this")
		}
	}

	// running backup again does not error out
	err = rw.Backup()
	c.Assert(err, IsNil)
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterBackupWithDirectories(c *C) {
	// some data for the gadget
	gdWritten := []gadgetData{
		{name: "bar", content: "data"},
		{name: "some-dir/foo", content: "data"},
		{name: "empty-dir/"},
	}
	makeGadgetData(c, s.dir, gdWritten)

	outDir := filepath.Join(c.MkDir(), "out-dir")

	// these exist in the destination directory and will be backed up
	backedUp := []gadgetData{
		// overwritten by "bar" -> "/foo"
		{target: "foo", content: "can't touch this"},
		// overwritten by some-dir/ -> /nested/
		{target: "nested/foo", content: "can't touch this"},
		// written to by bar -> /this/is/some/nested/
		{target: "this/is/some/"},
		{target: "lone-dir/"},
	}
	makeExistingData(c, outDir, backedUp)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				}, {
					Source: "bar",
					Target: "/this/is/some/nested/",
				}, {
					Source: "some-dir/",
					Target: "/nested/",
				}, {
					Source: "empty-dir/",
					Target: "/lone-dir/",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition:  1,
				Preserve: []string{"/zed"},
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Backup()
	c.Assert(err, IsNil)

	verifyDirContents(c, filepath.Join(s.backup, "struct-0"), map[string]contentType{
		"this/is/some.backup": typeFile,
		"this/is.backup":      typeFile,
		"this.backup":         typeFile,

		"nested/foo.backup": typeFile,
		"nested.backup":     typeFile,

		"foo.backup": typeFile,

		"lone-dir.backup": typeFile,
	})
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterBackupNonexistent(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{"bar", "foo", "data"},
		{"bar", "some-dir/foo", "data"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := filepath.Join(c.MkDir(), "out-dir")

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				}, {
					Source: "bar",
					Target: "/some-dir/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
				// bar not in preserved files
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Backup()
	c.Assert(err, IsNil)

	backupRoot := filepath.Join(s.backup, "struct-0")
	// actually empty
	verifyDirContents(c, backupRoot, map[string]contentType{})
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterBackupFailsOnBackupDirErrors(c *C) {
	outDir := filepath.Join(c.MkDir(), "out-dir")

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = os.Chmod(s.backup, 0555)
	c.Assert(err, IsNil)
	defer os.Chmod(s.backup, 0755)

	err = rw.Backup()
	c.Assert(err, ErrorMatches, "cannot create backup directory: .*/struct-0: permission denied")
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterBackupFailsOnDestinationErrors(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{name: "bar", content: "data"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "same"},
	})

	err := os.Chmod(filepath.Join(outDir, "foo"), 0000)
	c.Assert(err, IsNil)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Backup()
	c.Assert(err, ErrorMatches, "cannot backup content: cannot open destination file: open .*/out-dir/foo: permission denied")
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterBackupFailsOnBadSrcComparison(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{name: "bar", content: "data"},
	}
	makeGadgetData(c, s.dir, gd)
	err := os.Chmod(filepath.Join(s.dir, "bar"), 0000)
	c.Assert(err, IsNil)

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "same"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Backup()
	c.Assert(err, ErrorMatches, "cannot backup content: cannot checksum update file: open .*/bar: permission denied")
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterUpdate(c *C) {
	// some data for the gadget
	gdWritten := []gadgetData{
		{"foo", "foo-dir/foo", "data"},
		{"bar", "bar-name", "data"},
		{"boot-assets/splash", "splash", "data"},
		{"boot-assets/some-dir/data", "some-dir/data", "data"},
		{"boot-assets/some-dir/empty-file", "some-dir/empty-file", ""},
		{"boot-assets/nested-dir/more-nested/more", "/nested-copy/more-nested/more", "data"},
	}
	gdNotWritten := []gadgetData{
		{"foo", "/foo", "data"},
		{"boot-assets/some-dir/data", "data-copy", "data"},
		{"boot-assets/nested-dir/nested", "/nested-copy/nested", "data"},
	}
	makeGadgetData(c, s.dir, append(gdWritten, gdNotWritten...))

	// these exist in the root directory and are preserved
	preserve := []string{
		// mix entries with leading / and without
		"/foo",
		"/data-copy",
		"nested-copy/nested",
		"not-listed", // not present in 'gadget' contents
	}
	// these are preserved, but don't exist in the root, so data from gadget
	// will be written
	preserveButNotPresent := []string{
		"/bar-name",
		"some-dir/data",
	}
	outDir := filepath.Join(c.MkDir(), "out-dir")

	for _, en := range preserve {
		p := filepath.Join(outDir, en)
		makeSizedFile(c, p, 0, []byte("can't touch this"))
	}

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "foo",
					Target: "/foo-dir/",
				}, {
					// would overwrite /foo
					Source: "foo",
					Target: "/",
				}, {
					// preserved, but not present, will be
					// written
					Source: "bar",
					Target: "/bar-name",
				}, {
					// some-dir/data is preserved, but not
					// present, hence will be written
					Source: "boot-assets/",
					Target: "/",
				}, {
					// would overwrite /data-copy
					Source: "boot-assets/some-dir/data",
					Target: "/data-copy",
				}, {
					// would overwrite /nested-copy/nested
					Source: "boot-assets/nested-dir/",
					Target: "/nested-copy/",
				}, {
					Source: "boot-assets",
					Target: "/boot-assets-copy/",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition:  1,
				Preserve: append(preserve, preserveButNotPresent...),
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Backup()
	c.Assert(err, IsNil)

	err = rw.Update()
	c.Assert(err, IsNil)

	// files that existed were preserved
	for _, en := range preserve {
		p := filepath.Join(outDir, en)
		c.Check(p, testutil.FileEquals, "can't touch this")
	}
	// everything else was written
	verifyWrittenGadgetData(c, outDir, gdWritten)
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterUpdateLookupFails(c *C) {
	makeGadgetData(c, s.dir, []gadgetData{
		{"canary", "canary", "data"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "/",
					Target: "/",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return "", errors.New("failed")
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Update()
	c.Assert(err, ErrorMatches, "cannot find mount location of structure #0: failed")
}
func (s *mountedfilesystemTestSuite) TestMountedUpdaterDirContents(c *C) {
	// some data for the gadget
	gdWritten := []gadgetData{
		{"bar/foo", "/bar-name/foo", "data"},
		{"bar/nested/foo", "/bar-name/nested/foo", "data"},
		{"bar/foo", "/bar-copy/bar/foo", "data"},
		{"bar/nested/foo", "/bar-copy/bar/nested/foo", "data"},
		{"deep-nested", "/this/is/some/deep/nesting/deep-nested", "data"},
	}
	makeGadgetData(c, s.dir, gdWritten)

	outDir := filepath.Join(c.MkDir(), "out-dir")

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					// contents of bar under /bar-name/
					Source: "bar/",
					Target: "/bar-name",
				}, {
					// whole bar under /bar-copy/
					Source: "bar",
					Target: "/bar-copy/",
				}, {
					// deep prefix
					Source: "deep-nested",
					Target: "/this/is/some/deep/nesting/",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Backup()
	c.Assert(err, IsNil)

	err = rw.Update()
	c.Assert(err, IsNil)

	verifyWrittenGadgetData(c, outDir, gdWritten)
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterExpectsBackup(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{"bar", "foo", "update"},
		{"bar", "some-dir/foo", "update"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "content"},
		{target: "some-dir/foo", content: "content"},
		{target: "/preserved", content: "preserve"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				}, {
					Source: "bar",
					Target: "/some-dir/foo",
				}, {
					Source: "bar",
					Target: "/preserved",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
				// bar not in preserved files
				Preserve: []string{"preserved"},
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Update()
	c.Assert(err, ErrorMatches, `cannot update content: missing backup file ".*/struct-0/foo.backup" for /foo`)
	// create a mock backup of first file
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/foo.backup"), 0, nil)
	// try again
	err = rw.Update()
	c.Assert(err, ErrorMatches, `cannot update content: missing backup file ".*/struct-0/some-dir/foo.backup" for /some-dir/foo`)
	// create a mock backup of second entry
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/some-dir/foo.backup"), 0, nil)
	// try again (preserved files need no backup)
	err = rw.Update()
	c.Assert(err, IsNil)

	verifyWrittenGadgetData(c, outDir, []gadgetData{
		{target: "foo", content: "update"},
		{target: "some-dir/foo", content: "update"},
		{target: "/preserved", content: "preserve"},
	})
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterEmptyDir(c *C) {
	// some data for the gadget
	err := os.MkdirAll(filepath.Join(s.dir, "empty-dir"), 0755)
	c.Assert(err, IsNil)
	err = os.MkdirAll(filepath.Join(s.dir, "non-empty/empty-dir"), 0755)
	c.Assert(err, IsNil)

	outDir := filepath.Join(c.MkDir(), "out-dir")

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "/",
					Target: "/",
				}, {
					Source: "/",
					Target: "/foo",
				}, {
					Source: "/non-empty/empty-dir/",
					Target: "/contents-of-empty/",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Update()
	c.Assert(err, IsNil)

	verifyDirContents(c, outDir, map[string]contentType{
		// / -> /
		"empty-dir":           typeDir,
		"non-empty/empty-dir": typeDir,

		// / -> /foo
		"foo/empty-dir":           typeDir,
		"foo/non-empty/empty-dir": typeDir,

		// /non-empty/empty-dir/ -> /contents-of-empty/
		"contents-of-empty": typeDir,
	})
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterSameFileSkipped(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{"bar", "foo", "data"},
		{"bar", "some-dir/foo", "data"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "same"},
		{target: "some-dir/foo", content: "same"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				}, {
					Source: "bar",
					Target: "/some-dir/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	// pretend a backup pass ran and found the files identical
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/foo.same"), 0, nil)
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/some-dir/foo.same"), 0, nil)

	err = rw.Update()
	c.Assert(err, IsNil)
	// files were not modified
	verifyWrittenGadgetData(c, outDir, []gadgetData{
		{target: "foo", content: "same"},
		{target: "some-dir/foo", content: "same"},
	})
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterLonePrefix(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{name: "bar", target: "1/nested/bar", content: "data"},
		{name: "bar", target: "2/nested/foo", content: "data"},
		{name: "bar", target: "3/nested/bar", content: "data"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := filepath.Join(c.MkDir(), "out-dir")

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/1/nested/",
				}, {
					Source: "bar",
					Target: "/2/nested/foo",
				}, {
					Source: "/",
					Target: "/3/nested/",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Update()
	c.Assert(err, IsNil)
	verifyWrittenGadgetData(c, outDir, gd)
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterRollbackFromBackup(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{"bar", "foo", "data"},
		{"bar", "some-dir/foo", "data"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "written"},
		{target: "some-dir/foo", content: "written"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				}, {
					Source: "bar",
					Target: "/some-dir/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	// pretend a backup pass ran and created a backup
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/foo.backup"), 0, []byte("backup"))
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/some-dir/foo.backup"), 0, []byte("backup"))

	err = rw.Rollback()
	c.Assert(err, IsNil)
	// files were restored from backup
	verifyWrittenGadgetData(c, outDir, []gadgetData{
		{target: "foo", content: "backup"},
		{target: "some-dir/foo", content: "backup"},
	})
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterRollbackSkipSame(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{name: "bar", content: "data"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "same"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	// pretend a backup pass ran and created a backup
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/foo.same"), 0, nil)

	err = rw.Rollback()
	c.Assert(err, IsNil)
	// files were not modified
	verifyWrittenGadgetData(c, outDir, []gadgetData{
		{target: "foo", content: "same"},
	})
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterRollbackSkipPreserved(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{name: "bar", content: "data"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "preserved"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition:  1,
				Preserve: []string{"foo"},
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	// preserved files get no backup, but gets a stamp instead
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/foo.preserve"), 0, nil)

	err = rw.Rollback()
	c.Assert(err, IsNil)
	// files were not modified
	verifyWrittenGadgetData(c, outDir, []gadgetData{
		{target: "foo", content: "preserved"},
	})
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterRollbackNewFiles(c *C) {
	makeGadgetData(c, s.dir, []gadgetData{
		{name: "bar", content: "data"},
	})

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "written"},
		{target: "some-dir/bar", content: "written"},
		{target: "this/is/some/deep/nesting/bar", content: "written"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				}, {
					Source: "bar",
					Target: "some-dir/",
				}, {
					Source: "bar",
					Target: "/this/is/some/deep/nesting/",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	// none of the marker files exists, files are new, will be removed
	err = rw.Rollback()
	c.Assert(err, IsNil)
	// everything was removed
	verifyDirContents(c, outDir, map[string]contentType{})
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterRollbackRestoreFails(c *C) {
	makeGadgetData(c, s.dir, []gadgetData{
		{name: "bar", content: "data"},
	})

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "written"},
		{target: "some-dir/foo", content: "written"},
	})
	// make rollback fail when restoring
	err := os.Chmod(filepath.Join(outDir, "foo"), 0000)
	c.Assert(err, IsNil)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				}, {
					Source: "bar",
					Target: "/some-dir/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	// one file backed up, the other is new
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/foo.backup"), 0, []byte("backup"))

	err = rw.Rollback()
	c.Assert(err, ErrorMatches, "cannot rollback content: cannot copy .*: unable to create .*/out-dir/foo: permission denied")

	// remove offending file
	c.Assert(os.Remove(filepath.Join(outDir, "foo")), IsNil)

	// make destination dir non-writable
	err = os.Chmod(filepath.Join(outDir, "some-dir"), 0555)
	c.Assert(err, IsNil)
	// restore permissions later, otherwise test suite cleanup complains
	defer os.Chmod(filepath.Join(outDir, "some-dir"), 0755)

	err = rw.Rollback()
	c.Assert(err, ErrorMatches, "cannot rollback content: cannot remove written update: remove .*/out-dir/some-dir/foo: permission denied")
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterRollbackNotWritten(c *C) {
	makeGadgetData(c, s.dir, []gadgetData{
		{name: "bar", content: "data"},
	})

	outDir := filepath.Join(c.MkDir(), "out-dir")

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "bar",
					Target: "/foo",
				}, {
					Source: "bar",
					Target: "/some-dir/foo",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	// rollback does not error out if files were not written
	err = rw.Rollback()
	c.Assert(err, IsNil)
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterRollbackDirectory(c *C) {
	makeGadgetData(c, s.dir, []gadgetData{
		{name: "some-dir/bar", content: "data"},
		{name: "some-dir/foo", content: "data"},
		{name: "some-dir/nested/nested-foo", content: "data"},
		{name: "empty-dir/"},
	})

	outDir := filepath.Join(c.MkDir(), "out-dir")
	makeExistingData(c, outDir, []gadgetData{
		// some-dir/ -> /
		{target: "foo", content: "written"},
		{target: "bar", content: "written"},
		{target: "nested/nested-foo", content: "written"},
		// some-dir/ -> /other-dir/
		{target: "other-dir/foo", content: "written"},
		{target: "other-dir/bar", content: "written"},
		{target: "other-dir/nested/nested-foo", content: "written"},
		// some-dir/nested -> /other-dir/nested/
		{target: "other-dir/nested/nested/nested-foo", content: "written"},
		// bar -> /this/is/some/deep/nesting/
		{target: "this/is/some/deep/nesting/bar", content: "written"},
		{target: "lone-dir/"},
	})

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "some-dir/",
					Target: "/",
				}, {
					Source: "some-dir/",
					Target: "/other-dir/",
				}, {
					Source: "some-dir/nested",
					Target: "/other-dir/nested/",
				}, {
					Source: "bar",
					Target: "/this/is/some/deep/nesting/",
				}, {
					Source: "empty-dir/",
					Target: "/lone-dir/",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition: 1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	// one file backed up
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/foo.backup"), 0, []byte("backup"))
	// pretend part of the directory structure existed before
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/this/is/some.backup"), 0, nil)
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/this/is.backup"), 0, nil)
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/this.backup"), 0, nil)
	makeSizedFile(c, filepath.Join(s.backup, "struct-0/lone-dir.backup"), 0, nil)

	// files without a marker are new, will be removed
	err = rw.Rollback()
	c.Assert(err, IsNil)

	verifyDirContents(c, outDir, map[string]contentType{
		"lone-dir":     typeDir,
		"this/is/some": typeDir,
		"foo":          typeFile,
	})
	// this one got restored
	c.Check(filepath.Join(outDir, "foo"), testutil.FileEquals, "backup")

}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterEndToEndOne(c *C) {
	// some data for the gadget
	gdWritten := []gadgetData{
		{"foo", "foo-dir/foo", "data"},
		{"bar", "bar-name", "data"},
		{"boot-assets/splash", "splash", "data"},
		{"boot-assets/some-dir/data", "some-dir/data", "data"},
		{"boot-assets/some-dir/empty-file", "some-dir/empty-file", ""},
		{"boot-assets/nested-dir/more-nested/more", "/nested-copy/more-nested/more", "data"},
	}
	gdNotWritten := []gadgetData{
		{"foo", "/foo", "data"},
		{"boot-assets/some-dir/data", "data-copy", "data"},
		{"boot-assets/nested-dir/nested", "/nested-copy/nested", "data"},
	}
	makeGadgetData(c, s.dir, append(gdWritten, gdNotWritten...))
	err := os.MkdirAll(filepath.Join(s.dir, "boot-assets/empty-dir"), 0755)
	c.Assert(err, IsNil)

	outDir := filepath.Join(c.MkDir(), "out-dir")

	makeExistingData(c, outDir, []gadgetData{
		{target: "foo", content: "can't touch this"},
		{target: "data-copy-preserved", content: "can't touch this"},
		{target: "data-copy", content: "can't touch this"},
		{target: "nested-copy/nested", content: "can't touch this"},
		{target: "nested-copy/more-nested/"},
		{target: "not-listed", content: "can't touch this"},
		{target: "unrelated/data/here", content: "unrelated"},
	})
	// these exist in the root directory and are preserved
	preserve := []string{
		// mix entries with leading / and without
		"/foo",
		"/data-copy-preserved",
		"nested-copy/nested",
		"not-listed", // not present in 'gadget' contents
	}
	// these are preserved, but don't exist in the root, so data from gadget
	// will be written
	preserveButNotPresent := []string{
		"/bar-name",
		"some-dir/data",
	}

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{
					Source: "foo",
					Target: "/foo-dir/",
				}, {
					// would overwrite /foo
					Source: "foo",
					Target: "/",
				}, {
					// preserved, but not present, will be
					// written
					Source: "bar",
					Target: "/bar-name",
				}, {
					// some-dir/data is preserved, but not
					// present, hence will be written
					Source: "boot-assets/",
					Target: "/",
				}, {
					// would overwrite /data-copy
					Source: "boot-assets/some-dir/data",
					Target: "/data-copy-preserved",
				}, {
					Source: "boot-assets/some-dir/data",
					Target: "/data-copy",
				}, {
					// would overwrite /nested-copy/nested
					Source: "boot-assets/nested-dir/",
					Target: "/nested-copy/",
				}, {
					Source: "boot-assets",
					Target: "/boot-assets-copy/",
				}, {
					Source: "/boot-assets/empty-dir/",
					Target: "/lone-dir/nested/",
				},
			},
			Update: gadget.VolumeUpdate{
				Edition:  1,
				Preserve: append(preserve, preserveButNotPresent...),
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	originalState := map[string]contentType{
		"foo":                     typeFile,
		"data-copy":               typeFile,
		"not-listed":              typeFile,
		"data-copy-preserved":     typeFile,
		"nested-copy/nested":      typeFile,
		"nested-copy/more-nested": typeDir,
		"unrelated/data/here":     typeFile,
	}
	verifyDirContents(c, outDir, originalState)

	// run the backup phase
	err = rw.Backup()
	c.Assert(err, IsNil)

	verifyDirContents(c, filepath.Join(s.backup, "struct-0"), map[string]contentType{
		"nested-copy.backup":             typeFile,
		"nested-copy/nested.preserve":    typeFile,
		"nested-copy/more-nested.backup": typeFile,
		"foo.preserve":                   typeFile,
		"data-copy-preserved.preserve":   typeFile,
		"data-copy.backup":               typeFile,
	})

	// run the update phase
	err = rw.Update()
	c.Assert(err, IsNil)

	verifyDirContents(c, outDir, map[string]contentType{
		"foo":        typeFile,
		"not-listed": typeFile,

		// boot-assets/some-dir/data -> /data-copy
		"data-copy": typeFile,

		// boot-assets/some-dir/data -> /data-copy-preserved
		"data-copy-preserved": typeFile,

		// foo -> /foo-dir/
		"foo-dir/foo": typeFile,

		// bar -> /bar-name
		"bar-name": typeFile,

		// boot-assets/ -> /
		"splash":                      typeFile,
		"some-dir/data":               typeFile,
		"some-dir/empty-file":         typeFile,
		"nested-dir/nested":           typeFile,
		"nested-dir/more-nested/more": typeFile,
		"empty-dir":                   typeDir,

		// boot-assets -> /boot-assets-copy/
		"boot-assets-copy/boot-assets/splash":                      typeFile,
		"boot-assets-copy/boot-assets/some-dir/data":               typeFile,
		"boot-assets-copy/boot-assets/some-dir/empty-file":         typeFile,
		"boot-assets-copy/boot-assets/nested-dir/nested":           typeFile,
		"boot-assets-copy/boot-assets/nested-dir/more-nested/more": typeFile,
		"boot-assets-copy/boot-assets/empty-dir":                   typeDir,

		// boot-assets/nested-dir/ -> /nested-copy/
		"nested-copy/nested":           typeFile,
		"nested-copy/more-nested/more": typeFile,

		// data that was not part of the update
		"unrelated/data/here": typeFile,

		// boot-assets/empty-dir/ -> /lone-dir/nested/
		"lone-dir/nested": typeDir,
	})

	// files that existed were preserved
	for _, en := range preserve {
		p := filepath.Join(outDir, en)
		c.Check(p, testutil.FileEquals, "can't touch this")
	}
	// everything else was written
	verifyWrittenGadgetData(c, outDir, gdWritten)

	err = rw.Rollback()
	c.Assert(err, IsNil)
	// back to square one
	verifyDirContents(c, outDir, originalState)
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterTrivialValidation(c *C) {
	psNoFs := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size: 2048,
			// no filesystem
			Content: []gadget.VolumeContent{},
		},
	}

	lookupFail := func(to *gadget.PositionedStructure) (string, error) {
		c.Fatalf("unexpected call")
		return "", nil
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, psNoFs, s.backup, lookupFail)
	c.Assert(err, ErrorMatches, "structure #0 has no filesystem")
	c.Assert(rw, IsNil)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content:    []gadget.VolumeContent{},
		},
	}

	rw, err = gadget.NewMountedFilesystemUpdater("", ps, s.backup, lookupFail)
	c.Assert(err, ErrorMatches, `internal error: gadget content directory cannot be unset`)
	c.Assert(rw, IsNil)

	rw, err = gadget.NewMountedFilesystemUpdater(s.dir, ps, "", lookupFail)
	c.Assert(err, ErrorMatches, `internal error: backup directory must not be unset`)
	c.Assert(rw, IsNil)

	rw, err = gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, nil)
	c.Assert(err, ErrorMatches, `internal error: mount lookup helper must be provided`)
	c.Assert(rw, IsNil)

	rw, err = gadget.NewMountedFilesystemUpdater(s.dir, nil, s.backup, lookupFail)
	c.Assert(err, ErrorMatches, `internal error: \*PositionedStructure.*`)
	c.Assert(rw, IsNil)

	lookupOk := func(to *gadget.PositionedStructure) (string, error) {
		return filepath.Join(s.dir, "foobar"), nil
	}

	for _, tc := range []struct {
		content gadget.VolumeContent
		match   string
	}{
		{content: gadget.VolumeContent{Source: "", Target: "/"}, match: "internal error: source cannot be unset"},
		{content: gadget.VolumeContent{Source: "/", Target: ""}, match: "internal error: target cannot be unset"},
	} {
		testPs := &gadget.PositionedStructure{
			VolumeStructure: &gadget.VolumeStructure{
				Size:       2048,
				Filesystem: "ext4",
				Content:    []gadget.VolumeContent{tc.content},
			},
		}

		rw, err := gadget.NewMountedFilesystemUpdater(s.dir, testPs, s.backup, lookupOk)
		c.Assert(err, IsNil)
		c.Assert(rw, NotNil)

		err = rw.Update()
		c.Assert(err, ErrorMatches, "cannot update content: "+tc.match)

		err = rw.Backup()
		c.Assert(err, ErrorMatches, "cannot backup content: "+tc.match)

		err = rw.Rollback()
		c.Assert(err, ErrorMatches, "cannot rollback content: "+tc.match)
	}
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterMountLookupFail(c *C) {
	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{Source: "/", Target: "/"},
			},
		},
	}

	lookupFail := func(to *gadget.PositionedStructure) (string, error) {
		return "", errors.New("fail fail fail")
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, lookupFail)
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Update()
	c.Assert(err, ErrorMatches, "cannot find mount location of structure #0: fail fail fail")

	err = rw.Backup()
	c.Assert(err, ErrorMatches, "cannot find mount location of structure #0: fail fail fail")

	err = rw.Rollback()
	c.Assert(err, ErrorMatches, "cannot find mount location of structure #0: fail fail fail")
}

func (s *mountedfilesystemTestSuite) TestMountedUpdaterNonFilePreserveError(c *C) {
	// some data for the gadget
	gd := []gadgetData{
		{name: "foo", content: "data"},
	}
	makeGadgetData(c, s.dir, gd)

	outDir := filepath.Join(c.MkDir(), "out-dir")
	// will conflict with preserve entry
	err := os.MkdirAll(filepath.Join(outDir, "foo"), 0755)
	c.Assert(err, IsNil)

	ps := &gadget.PositionedStructure{
		VolumeStructure: &gadget.VolumeStructure{
			Size:       2048,
			Filesystem: "ext4",
			Content: []gadget.VolumeContent{
				{Source: "/", Target: "/"},
			},
			Update: gadget.VolumeUpdate{
				Preserve: []string{"foo"},
				Edition:  1,
			},
		},
	}

	rw, err := gadget.NewMountedFilesystemUpdater(s.dir, ps, s.backup, func(to *gadget.PositionedStructure) (string, error) {
		c.Check(to, DeepEquals, ps)
		return outDir, nil
	})
	c.Assert(err, IsNil)
	c.Assert(rw, NotNil)

	err = rw.Backup()
	c.Check(err, ErrorMatches, `cannot map preserve entries for mount location ".*/out-dir": preserved entry "foo" is a directory`)
	err = rw.Update()
	c.Check(err, ErrorMatches, `cannot map preserve entries for mount location ".*/out-dir": preserved entry "foo" is a directory`)
	err = rw.Rollback()
	c.Check(err, ErrorMatches, `cannot map preserve entries for mount location ".*/out-dir": preserved entry "foo" is a directory`)
}
