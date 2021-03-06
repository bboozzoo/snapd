/*
 * Copyright (C) 2017 Canonical Ltd
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

package hookstate

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/snapcore/snapd/i18n"
	"github.com/snapcore/snapd/overlord/snapstate"
	"github.com/snapcore/snapd/overlord/state"
)

func init() {
	snapstate.SetupInstallHook = SetupInstallHook
	snapstate.SetupPreRefreshHook = SetupPreRefreshHook
	snapstate.SetupPostRefreshHook = SetupPostRefreshHook
	snapstate.SetupRemoveHook = SetupRemoveHook
	snapstate.SetupGateAutoRefreshHook = SetupGateAutoRefreshHook
}

func SetupInstallHook(st *state.State, snapName string) *state.Task {
	hooksup := &HookSetup{
		Snap:     snapName,
		Hook:     "install",
		Optional: true,
	}

	summary := fmt.Sprintf(i18n.G("Run install hook of %q snap if present"), hooksup.Snap)
	task := HookTask(st, summary, hooksup, nil)

	return task
}

func SetupPostRefreshHook(st *state.State, snapName string) *state.Task {
	hooksup := &HookSetup{
		Snap:     snapName,
		Hook:     "post-refresh",
		Optional: true,
	}

	summary := fmt.Sprintf(i18n.G("Run post-refresh hook of %q snap if present"), hooksup.Snap)
	return HookTask(st, summary, hooksup, nil)
}

func SetupPreRefreshHook(st *state.State, snapName string) *state.Task {
	hooksup := &HookSetup{
		Snap:     snapName,
		Hook:     "pre-refresh",
		Optional: true,
	}

	summary := fmt.Sprintf(i18n.G("Run pre-refresh hook of %q snap if present"), hooksup.Snap)
	task := HookTask(st, summary, hooksup, nil)

	return task
}

func SetupGateAutoRefreshHook(st *state.State, snapName string, base, restart bool, affectingSnaps map[string]bool) *state.Task {
	hookSup := &HookSetup{
		Snap:     snapName,
		Hook:     "gate-auto-refresh",
		Optional: true,
	}
	affecting := make([]string, 0, len(affectingSnaps))
	for sn := range affectingSnaps {
		affecting = append(affecting, sn)
	}
	sort.Strings(affecting)
	summary := fmt.Sprintf(i18n.G("Run hook %s of snap %q"), hookSup.Hook, hookSup.Snap)
	hookCtx := map[string]interface{}{
		"base":            base,
		"restart":         restart,
		"affecting-snaps": affecting,
	}
	task := HookTask(st, summary, hookSup, hookCtx)
	return task
}

type snapHookHandler struct {
}

func (h *snapHookHandler) Before() error {
	return nil
}

func (h *snapHookHandler) Done() error {
	return nil
}

func (h *snapHookHandler) Error(err error) (bool, error) {
	return false, nil
}

func SetupRemoveHook(st *state.State, snapName string) *state.Task {
	hooksup := &HookSetup{
		Snap:        snapName,
		Hook:        "remove",
		Optional:    true,
		IgnoreError: true,
	}

	summary := fmt.Sprintf(i18n.G("Run remove hook of %q snap if present"), hooksup.Snap)
	task := HookTask(st, summary, hooksup, nil)

	return task
}

func setupHooks(hookMgr *HookManager) {
	handlerGenerator := func(context *Context) Handler {
		return &snapHookHandler{}
	}

	hookMgr.Register(regexp.MustCompile("^install$"), handlerGenerator)
	hookMgr.Register(regexp.MustCompile("^post-refresh$"), handlerGenerator)
	hookMgr.Register(regexp.MustCompile("^pre-refresh$"), handlerGenerator)
	hookMgr.Register(regexp.MustCompile("^remove$"), handlerGenerator)
	hookMgr.Register(regexp.MustCompile("^gate-auto-refresh$"), handlerGenerator)
}
