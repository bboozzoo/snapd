package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	baseDir         = "/tmp/snappy-test"
	debsTestBedPath = "/tmp/snappy-debs"
	defaultRelease  = "15.04"
	defaultChannel  = "edge"
)

var (
	debsDir     = filepath.Join(baseDir, "debs")
	imageDir    = filepath.Join(baseDir, "image")
	outputDir   = filepath.Join(baseDir, "output")
	imageTarget = filepath.Join(imageDir, "snappy.img")
)

func execCommand(cmds ...string) {
	cmd := exec.Command(cmds[0], cmds[1:len(cmds)]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func buildDebs(rootPath string) {
	fmt.Println("Building debs...")
	prepareTargetDir(debsDir)
	execCommand(
		"bzr", "bd",
		fmt.Sprintf("--result-dir=%s", debsDir),
		rootPath,
		"--", "-uc", "-us")
}

func createImage(release, channel string) {
	fmt.Println("Creating image...")
	prepareTargetDir(imageDir)
	execCommand(
		"sudo", "ubuntu-device-flash", "--verbose",
		"core", release,
		"-o", imageTarget,
		"--channel", channel,
		"--developer-mode")
}

func adtRun(rootPath string) {
	fmt.Println("Calling adt-run...")
	prepareTargetDir(outputDir)
	execCommand(
		"adt-run",
		"-B",
		"--setup-commands", "touch /run/autopkgtest_no_reboot.stamp",
		"--setup-commands", "mount -o remount,rw /",
		"--setup-commands",
		fmt.Sprintf("dpkg -i %s/*deb", debsTestBedPath),
		"--setup-commands",
		"sync; sleep 2; mount -o remount,ro /",
		"--override-control", "debian/integration-tests/control",
		"--built-tree", rootPath,
		"--output-dir", outputDir,
		fmt.Sprintf("--copy=%s:%s", debsDir, debsTestBedPath),
		"---",
		"ssh", "-s",
		"/usr/share/autopkgtest/ssh-setup/snappy",
		"--", "-i", imageTarget)
}

func prepareTargetDir(targetDir string) {
	_, err := os.Stat(targetDir)
	if err == nil {
		// dir exists, remove it
		os.RemoveAll(targetDir)
	}
	os.MkdirAll(targetDir, 0777)
}

func getRootPath() string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	return dir
}

func main() {
	rootPath := getRootPath()

	buildDebs(rootPath)

	createImage(defaultRelease, defaultChannel)

	adtRun(rootPath)
}
