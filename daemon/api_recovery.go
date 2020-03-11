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

package daemon

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/snapcore/snapd/client"
	"github.com/snapcore/snapd/overlord/auth"
)

var recoverySystemsCmd = &Command{
	Path: "/v2/recovery",
	GET:  getRecoverySystems,
}

var recoverFromCmd = &Command{
	Path: "/v2/recover/{label}",
	POST: postRecoverSystem,
}

func getRecoverySystems(c *Command, r *http.Request, user *auth.UserState) Response {
	deviceMgr := c.d.overlord.DeviceManager()

	labels, err := deviceMgr.RecoverySystems()
	if err != nil {
		return InternalError(err)
	}
	return SyncResponse(nil, nil)
}

func postRecoverSystem(c *Command, r *http.Request, user *auth.UserState) Response {
	vars := muxVars(r)
	systemLabel := vars["label"]

	var action client.RecoverySystemAction

	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&inst); err != nil {
		return BadRequest("cannot decode request body recovery system action: %v", err)
	}
	if dec.More() {
		return BadRequest("spurious content after recovery system action")
	}
	if action.ID == "" {
		return BadRequest("cannot recover without action ID")
	}

	deviceMgr := c.d.overlord.DeviceManager()
	if err := deviceMgr.RecoverFromSystem(label); err != nil {
		return InternalError(err)
	}
	return SyncResponse(nil, nil)
}
