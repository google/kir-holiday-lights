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
	"strings"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// settingsSection is the ordinary FPP settings the config names, by FPP's
// own names. Each entry is "settings.<Name>". Settings another section owns
// are not in cfg.Settings (the schema keeps them apart).
type settingsSection struct {
	cfg *config.Controller
	// defs is each configured setting's definition, from Observe.
	defs  map[string]target.Setting
	notes []string
}

func NewSettings(cfg *config.Controller) Section { return &settingsSection{cfg: cfg} }

func (s *settingsSection) Name() string { return "settings" }

func settingsKey(name string) string { return "settings." + name }

func (s *settingsSection) Want() state.State {
	var out state.State
	for _, st := range s.cfg.Settings {
		out = append(out, state.Entry{Key: settingsKey(st.Name), Value: st.Value})
	}
	return out
}

// Observe reads each configured setting and its definition; see
// settingsObserve.
func (s *settingsSection) Observe(ctx context.Context, t *target.Target) (state.State, error) {
	st, defs, notes, err := settingsObserve(ctx, t.API, s.cfg.Settings, settingsKey)
	s.defs, s.notes = defs, notes
	return st, err
}

func (s *settingsSection) Notes() []string { return s.notes }

// Apply writes each changed setting, in the order push printed them (config
// order), and asks for the most any of them needs.
func (s *settingsSection) Apply(ctx context.Context, t *target.Target, changes []state.Change) (After, error) {
	after := None
	for _, c := range changes {
		name := strings.TrimPrefix(c.Key, "settings.")
		if err := target.PutSetting(ctx, t.API, name, *c.Want); err != nil {
			return after, fmt.Errorf("writing setting %s: %w", name, err)
		}
		after = max(after, settingsAfter(s.defs[name]))
	}
	return after, nil
}

// Unset lists the managed settings the config leaves unset, with their
// current values. Those this FPP lacks are left out.
func (s *settingsSection) Unset(ctx context.Context, t *target.Target) (state.State, error) {
	set := make(map[string]bool, len(s.cfg.Settings))
	for _, st := range s.cfg.Settings {
		set[st.Name] = true
	}
	var out state.State
	for _, name := range config.ManagedSettings() {
		if set[name] {
			continue
		}
		def, err := target.GetSetting(ctx, t.API, name)
		if err != nil {
			return nil, fmt.Errorf("reading setting %s: %w", name, err)
		}
		if !def.Defined {
			continue
		}
		out = append(out, state.Entry{Key: settingsKey(name), Value: def.Value})
	}
	return out, nil
}
