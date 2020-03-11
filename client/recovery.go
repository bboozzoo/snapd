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

package client

import (
	"bytes"
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"
)

type RecoverySystem struct {
	// Current is true when the system running now was installed from that
	// recovery seed
	Current bool `json:"current"`
	// Label of the recovery system
	Label string `json:"label"`
	// Model name
	Model string `json:"model"`
	// ModelDisplayName is the human friendly name
	ModelDisplayName string `json:"model-display-name"`
	// Brand of the model
	Brand   string                 `json:"brand"`
	Actions []RecoverySystemAction `json:"actions"`
}

type RecoverySystemAction struct {
	// ID used when referencing given action in recover requests
	ID string `json:"id"`
	// Title is a user presentable action description
	Title string `json:"title"`
	// Mode given action can be executed in
	Mode string `json:"mode"`
}

type Recovery struct {
	Systems []RecoverySystem `json:"systems"`
}

func (client *Client) ListRecoverySystems() ([]RecoverySystem, error) {
	var recovery Recovery

	if _, err := client.doSync("GET", "/v2/recovery", nil, nil, nil, &recovery); err != nil {
		return nil, xerrors.Errorf("cannot list recovery systems: %v", err)
	}
	return recovery.Systems
}

type RecoverSystemAction struct {
	ID string `json:"id"`
	// XXX: already known through the endpoint path
}

func (client *Client) RecoveryAction(systemLabel string, actionID string) error {
	if systemLabel == "" || actionID == "" {
		return fmt.Errorf("cannot request recovery action with incomplete data")
	}

	data, err := json.Marshal(&RecoverySystemAction{ID: actionID})
	if err != nil {
		return fmt.Errorf("cannot create a recovery action request: %v", err)
	}

	if _, err := client.doSync("POST", "/v2/recover/"+systemLabel, nil, nil, bytes.NewReader(data), nil); err != nil {
		return xerrors.Errorf("cannot request a system recovery action: %v", err)
	}
	return nil
}
