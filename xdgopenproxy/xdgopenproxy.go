// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2018 Canonical Ltd
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

// Package xdgopenproxy provides a client for snap userd's xdg-open D-Bus proxy
package xdgopenproxy

import (
	"fmt"
	"net/url"
	"syscall"
	"time"

	"github.com/godbus/dbus"
	"golang.org/x/xerrors"
)

type bus interface {
	Object(name string, objectPath dbus.ObjectPath) dbus.BusObject
	AddMatchSignal(options ...dbus.MatchOption) error
	RemoveMatchSignal(options ...dbus.MatchOption) error
	Signal(ch chan<- *dbus.Signal)
	RemoveSignal(ch chan<- *dbus.Signal)
	Close() error
}

type userDeclinedError struct {
	msg string
}

func (u *userDeclinedError) Error() string { return u.msg }

func (u *userDeclinedError) Is(err error) bool {
	_, ok := err.(*userDeclinedError)
	return ok
}

var sessionBus = func() (bus, error) { return dbus.SessionBus() }

type desktopLauncher interface {
	openFile(path string) error
	openURL(url string) error
}

type portalLauncher struct {
	bus     bus
	service dbus.BusObject
}

func (p *portalLauncher) openFile(filename string) error {
	fd, err := syscall.Open(filename, syscall.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)

	return p.checkedCall("org.freedesktop.portal.OpenURI.OpenFile", 0, "", dbus.UnixFD(fd),
		map[string]dbus.Variant{})
}

func (p *portalLauncher) openURL(path string) error {
	return p.checkedCall("org.freedesktop.portal.OpenURI.OpenURI", 0, "", path,
		map[string]dbus.Variant{})
}

func (p *portalLauncher) checkedCall(member string, flags dbus.Flags, args ...interface{}) error {
	signals := make(chan *dbus.Signal)
	match := []dbus.MatchOption{
		dbus.WithMatchSender("org.freedesktop.portal.Desktop"),
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	}

	p.bus.Signal(signals)
	p.bus.AddMatchSignal(match...)

	defer func() {
		p.bus.RemoveMatchSignal(match...)
		p.bus.RemoveSignal(signals)
		close(signals)
	}()

	var handle dbus.ObjectPath
	if err := p.service.Call(member, flags, args...).Store(&handle); err != nil {
		return err
	}

	responseObject := p.bus.Object("org.freedesktop.portal.Desktop", handle)
	fmt.Printf("response object: %v\n", responseObject.Path())

	timeout := time.NewTicker(5 * time.Minute)
	defer timeout.Stop()
Loop:
	for {
		select {
		case <-timeout.C:
			if err := responseObject.Call("org.freedesktop.portal.Request.Close", 0); err != nil {
				return &userDeclinedError{msg: "timeout waiting for response"}
			}
			break Loop
		case signal := <-signals:
			fmt.Printf("got signal: %+v\n", signal)
			if signal.Path != responseObject.Path() {
				continue
			}

			var response uint
			var results map[string]interface{} // don't care
			if err := dbus.Store(signal.Body, &response, &results); err != nil {
				return &userDeclinedError{msg: fmt.Sprintf("cannot unpack response: %v", err)}
			}
			if response == 0 {
				return nil
			}
			return &userDeclinedError{msg: fmt.Sprintf("request declined by the user (code %v)", response)}
		}
	}

	return nil
}

func newPortalLauncher(bus bus) desktopLauncher {
	obj := bus.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")

	return &portalLauncher{service: obj, bus: bus}
}

type snapcraftLauncher struct {
	service dbus.BusObject
}

func (s *snapcraftLauncher) openFile(filename string) error {
	fd, err := syscall.Open(filename, syscall.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)

	return s.service.Call("io.snapcraft.Launcher.OpenFile", 0, "", dbus.UnixFD(fd)).Err
}

func (s *snapcraftLauncher) openURL(path string) error {
	return s.service.Call("io.snapcraft.Launcher.OpenURL", 0, path).Err
}

func newSnapcraftLauncher(bus bus) desktopLauncher {
	obj := bus.Object("io.snapcraft.Launcher", "/io/snapcraft/Launcher")

	return &snapcraftLauncher{service: obj}
}

func Run(urlOrFile string) error {
	sbus, err := sessionBus()
	if err != nil {
		return err
	}

	launchers := []desktopLauncher{newPortalLauncher(sbus), newSnapcraftLauncher(sbus)}

	defer sbus.Close()
	return launch(launchers, urlOrFile)
}

func launchWithOne(l desktopLauncher, urlOrFile string) error {
	if u, err := url.Parse(urlOrFile); err == nil {
		if u.Scheme == "file" {
			return l.openFile(u.Path)
		} else if u.Scheme != "" {
			return l.openURL(urlOrFile)
		}
	}
	return l.openFile(urlOrFile)
}

func launch(launchers []desktopLauncher, urlOrFile string) error {
	var err error
	for _, l := range launchers {
		err = launchWithOne(l, urlOrFile)
		fmt.Printf("launch error: %v\n", err)
		if err == nil {
			break
		}
		if xerrors.Is(err, &userDeclinedError{}) {
			break
		}
	}
	return err
}
