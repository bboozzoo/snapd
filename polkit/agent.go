// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2016-2017 Canonical Ltd
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

package polkit

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/logger"
	"github.com/snapcore/snapd/osutil/sys"
)

const (
	agentPath            = "/usr/bin/pkttyagent"
	agentRegisterTimeout = 30 * time.Second
)

type Agent struct {
	cmd *exec.Cmd
}

func StartAgent() (*Agent, error) {
	a := &Agent{}

	// TODO make sure there is only one agent

	if !terminal.IsTerminal(0) {
		// not an interactive session
		return a, nil
	}

	if sys.Geteuid() == sys.UserID(0) {
		// root does not require additional authorization
		return a, nil
	}

	if err := a.run(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Agent) run() error {
	path := filepath.Join(dirs.GlobalRootDir, agentPath)

	pid := os.Getpid()
	startTime, err := getStartTimeForPid(uint32(pid))
	if err != nil {
		return err
	}

	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to open pipe: %v", err)
	}
	defer r.Close()

	cmd := exec.Command(path,
		"--process", fmt.Sprintf("%v,%v", pid, startTime),
		"--notify-fd", "3",
		"--fallback",
	)

	// After go doc:
	//    ExtraFiles specifies additional open files to be inherited by the
	//    new process. It does not include standard input, standard output, or
	//    standard error. If non-nil, entry i becomes file descriptor 3+i.
	cmd.ExtraFiles = []*os.File{w}
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		w.Close()
		return err
	}

	a.cmd = cmd

	w.Close()

	registeredChan := make(chan bool, 1)
	go func() {
		b := make([]byte, 1)
		_, err := r.Read(b)
		if err == io.EOF {
			registeredChan <- true
		}
		close(registeredChan)
	}()

	timeout := time.After(agentRegisterTimeout)
	logger.Debugf("waiting to pkttyagent agent registration")
	select {
	case <-timeout:
		defer a.Close()
		return fmt.Errorf("timed out waiting for agent to register")
	case <-registeredChan:
	}
	return nil
}

func (a *Agent) Close() {
	if a.cmd == nil {
		// nothing to do
		return
	}

	a.cmd.Process.Kill()
	if err := a.cmd.Wait(); err != nil {
		logger.Debugf("agent exited with error %v", err)
	}
	a.cmd = nil
}
