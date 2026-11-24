// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package section

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// keysSection manages the SSH authorized_keys file FPP stores and installs
// for both root and fpp. It sees only that stored copy: a key put in the
// real files another way does not show, and a controller with no stored
// copy has no keys as far as the tool can tell. The file is replaced whole.
type keysSection struct {
	cfg *config.Controller
}

func NewKeys(cfg *config.Controller) Section {
	return &keysSection{cfg: cfg}
}

const keysPath = "/api/configfile/authorized_keys"

func (s *keysSection) Name() string { return "keys" }

func (s *keysSection) lines() []string {
	return netLines(strings.Join(s.cfg.SSHKeys, "\n"))
}

func (s *keysSection) Want() state.State {
	if !s.cfg.ManageKeys {
		return nil
	}
	return state.List("keys", s.lines())
}

// Observe reads the stored copy. A 404 means there is none.
func (s *keysSection) Observe(ctx context.Context, t *target.Target) (state.State, error) {
	resp, err := t.API.Do(ctx, http.MethodGet, keysPath, nil, "")
	if err != nil {
		return nil, fmt.Errorf("reading the stored authorized_keys: %w", err)
	}
	switch {
	case resp.Status == http.StatusNotFound:
		return state.List("keys", nil), nil
	case !resp.OK():
		return nil, &target.StatusError{Method: http.MethodGet, Path: keysPath, Status: resp.Status}
	}
	return state.List("keys", netLines(string(resp.Body))), nil
}

// Apply uploads the whole file. FPP stores it and copies it over root's and
// fpp's authorized_keys.
func (s *keysSection) Apply(ctx context.Context, t *target.Target, changes []state.Change) (After, error) {
	lines := s.lines()
	body := ""
	if len(lines) > 0 {
		body = strings.Join(lines, "\n") + "\n"
	}
	resp, err := t.API.Do(ctx, http.MethodPost, keysPath, strings.NewReader(body), "text/plain")
	if err == nil {
		err = target.CheckOK(http.MethodPost, keysPath, resp)
	}
	if err != nil {
		return None, fmt.Errorf("installing the keys: %w", err)
	}
	return None, nil
}
