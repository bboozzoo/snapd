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

package daemon

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/snapcore/snapd/overlord/auth"
	"github.com/snapcore/snapd/overlord/fdestate"
)

var systemSecurebootCmd = &Command{
	// TODO GET returning whether secure boot is relevant for the system?

	Path: "/v2/system/secureboot",
	POST: postSystemSecurebootAction,
	WriteAccess: interfaceRootAccess{
		// TODO should this be only allowed for services with fwupd on slot
		// side?
		Interfaces: []string{"fwupd"},
	},
}

func postSystemSecurebootAction(c *Command, r *http.Request, user *auth.UserState) Response {
	contentType := r.Header.Get("Content-Type")

	switch contentType {
	case "application/json":
		return postSystemSecurebootActionJSON(c, r)
	default:
		return BadRequest("unexpected content type: %q", contentType)
	}
}

func postSystemSecurebootActionJSON(c *Command, r *http.Request) Response {

	ucred, err := ucrednetGet(r.RemoteAddr)
	if err != nil {
		return Forbidden("cannot obtain credentials of the caller: %v", err)
	}

	var req struct {
		Action string           `json:"action,omitempty"`
		Data   *json.RawMessage `json:"data"`
	}

	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&req); err != nil {
		return BadRequest("cannot decode request body: %v", err)
	}
	if decoder.More() {
		return BadRequest("extra content found in request body")
	}

	switch req.Action {
	case "efi-secureboot-db-update":
		return postSystemActionEFISecurebootDBUpdate(c, req.Data, ucred)
	default:
		return BadRequest("unsupported action %q", req.Data)
	}
}

type efiDBUpdateAction struct {
	Subaction string `json:"subaction"`

	// The following fields are only relevant for 'prepare' action

	// Payload is a base64 encoded binary blob, hose are in the range from few
	// kB to tens of kBs
	Payload string `json:"payload,omitempty"`
	// DB indicates the secureboot keys DB which is a target of the action,
	// possible values are PK, KEK, DB, DBX
	DB string `json:"db,omitempty"`
}

func postSystemActionEFISecurebootDBUpdate(c *Command, raw *json.RawMessage, ucred *ucrednet) Response {
	// try to figure out identity if the caller
	// TODO: this should be allowed for non-snal calls too?
	snapName, err := cgroupSnapNameFromPid(int(ucred.Pid))
	if err != nil {
		return Forbidden("could not determine snap name for pid: %s", err)
	}

	if raw == nil || len(*raw) == 0 {
		return BadRequest("no request data provided")
	}

	var action efiDBUpdateAction
	if err := json.Unmarshal(*raw, &action); err != nil {
		return BadRequest("cannot decode efi-secureboot-db-update data: %v", err)
	}

	// TODO where do we get the identity from?
	identity := fdestate.EFIKeyManagerIdentity{
		SnapInstanceName: snapName,
	}

	switch action.Subaction {
	case "startup":
		if err := fdestate.EFISecureBootDBManagerStartup(c.d.state, identity); err != nil {
			return BadRequest("cannot notify of manager startup: %v", err)
		}
		return SyncResponse(nil)
	case "prepare":
		return postEFISecurebootDBUpdatePrepare(c, identity, &action)
	case "cleanup":
		if err := fdestate.EFISecureBootDBUpdateCleanup(c.d.state, identity); err != nil {
			return BadRequest("cannot notify of update cleanup: %v", err)
		}
		return SyncResponse(nil)
	default:
		return BadRequest("unsupported EFI secure boot DB update action: %q", action.Subaction)
	}
}

func postEFISecurebootDBUpdatePrepare(c *Command, identity fdestate.EFIKeyManagerIdentity, act *efiDBUpdateAction) Response {
	switch act.DB {
	case "DBX":
	case "PK", "KEK", "DB":
		return InternalError("not implemented")
	default:
		return BadRequest("incorrect key DB %q", act.DB)
	}

	// TODO check for compatibility with g_base64_encode()
	payload, err := base64.StdEncoding.DecodeString(act.Payload)
	if err != nil {
		return BadRequest("cannot decode payload: %v", err)
	}

	err = fdestate.EFISecureBootDBUpdatePrepare(c.d.state,
		fdestate.EFIKeyManagerIdentity{},
		fdestate.EFISecurebootDBX, // only DBX updates are supported
		payload)
	if err != nil {
		return BadRequest("cannot notify of update prepare: %v", err)
	}
	return SyncResponse(nil)
}
