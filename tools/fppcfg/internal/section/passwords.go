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

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// passwordsSection is the UI password and the fpp account's OS password,
// each with its enable flag. Only the flags are entries
// ("passwords.ui.enabled", "passwords.os.enabled"): the passwords themselves
// are not compared or shown, since FPP 10.4 does not return them. Push
// writes every password the config sets, on every run, and takes the API
// accepting it as done.
//
// No password ever goes into an error or a note.
type passwordsSection struct{ cfg *config.Controller }

func NewPasswords(cfg *config.Controller) Section { return &passwordsSection{cfg: cfg} }

func (s *passwordsSection) Name() string { return "passwords" }

// passwordsKind is one of the two passwords: its entry key, its FPP
// settings, and its value in the config.
type passwordsKind struct {
	key     string // "passwords.ui"
	setting string // "password"
	enable  string // "passwordEnable"
	value   string
}

func (s *passwordsSection) kinds() []passwordsKind {
	var out []passwordsKind
	if s.cfg.UIPassword != "" {
		out = append(out, passwordsKind{"passwords.ui", "password", "passwordEnable", s.cfg.UIPassword})
	}
	if s.cfg.OSPassword != "" {
		out = append(out, passwordsKind{"passwords.os", "osPassword", "osPasswordEnable", s.cfg.OSPassword})
	}
	return out
}

func (s *passwordsSection) Want() state.State {
	var out state.State
	for _, k := range s.kinds() {
		out = append(out, state.Entry{Key: k.key + ".enabled", Value: "1"})
	}
	return out
}

// Rewrites is true when the config sets a password.
func (s *passwordsSection) Rewrites() bool { return len(s.kinds()) > 0 }

// Observe reads the enable flags.
func (s *passwordsSection) Observe(ctx context.Context, t *target.Target) (state.State, error) {
	var out state.State
	for _, k := range s.kinds() {
		enabled, err := target.GetSetting(ctx, t.API, k.enable)
		if err != nil {
			return nil, fmt.Errorf("reading setting %s: %w", k.enable, err)
		}
		out = append(out, state.Entry{Key: k.key + ".enabled", Value: enabled.Value})
	}
	return out, nil
}

// Apply writes each password the config sets, then its enable flag if that
// changed. The password is always written first: enabling without it would
// set "falcon". The HTTP client is given the new UI password as soon as it
// is stored, so later requests authenticate with it once it is enabled.
func (s *passwordsSection) Apply(ctx context.Context, t *target.Target, changes []state.Change) (After, error) {
	changed := state.Changed(changes)
	for _, k := range s.kinds() {
		if err := target.PutSetting(ctx, t.API, k.setting, k.value); err != nil {
			return None, fmt.Errorf("writing setting %s: %w", k.setting, scrub(err, k.value))
		}
		if k.setting == "password" {
			t.API.AddPassword(k.value)
		}
		if changed[k.key+".enabled"] {
			if err := target.PutSetting(ctx, t.API, k.enable, "1"); err != nil {
				return None, fmt.Errorf("writing setting %s: %w", k.enable, scrub(err, k.value))
			}
		}
	}
	return None, nil
}
