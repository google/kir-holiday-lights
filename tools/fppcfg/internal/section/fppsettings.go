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
	"slices"
	"strings"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// Settings this FPP does not have.
//
// The engine takes Want from the config alone, before Observe has asked the
// controller anything, and compares once. So a section cannot drop a wanted
// key it later learns this FPP lacks. The two other ways out are both worse:
// leaving the key out of the observed state, or giving it a marker value
// such as "<not on this FPP>", makes a change that push prints, does nothing
// about, and prints again on every run.
//
// Instead the observed state carries the wanted value for such a key, so it
// is not a change, and the section records a note that the engine prints
// next to the diff. The entry then reads "not applicable here", not "set":
// the note is the record of it. Such a key is never a change, so Apply never
// sees it.

// settingsObserve reads each of list's settings and its definition, for the
// settings and setup sections; key makes a setting's entry key. A value the
// controller does not offer is an error here, not a refusal during push. A
// setting this FPP lacks is observed as the wanted value, with a note.
func settingsObserve(ctx context.Context, api target.API, list []config.Setting, key func(string) string) (st state.State, defs map[string]target.Setting, notes []string, err error) {
	defs = make(map[string]target.Setting, len(list))
	for _, want := range list {
		def, err := target.GetSetting(ctx, api, want.Name)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("reading setting %s: %w", want.Name, err)
		}
		defs[want.Name] = def
		have := def.Value // "" for a setting with no value and no default
		if !def.Defined {
			notes = append(notes, settingsSkipped(want.Name))
			have = want.Value
		} else if err := settingsCheckOption(want.Name, want.Value, def); err != nil {
			return nil, nil, nil, err
		}
		st = append(st, state.Entry{Key: key(want.Name), Value: have})
	}
	return st, defs, notes, nil
}

// settingsSkipped is the note for a setting this FPP does not have.
func settingsSkipped(name string) string {
	return name + ": not on this FPP; skipped"
}

// settingsAfter is what a change to a setting needs, from its definition.
// FPP's restart 1 is a full restart and 2 a quick one.
func settingsAfter(def target.Setting) After {
	switch {
	case def.Reboot == 1:
		return Reboot
	case def.Restart == 1:
		return Restart
	case def.Restart == 2:
		return QuickRestart
	}
	return None
}

// settingsCheckOption returns an error when def lists options and value is
// not one of their values.
func settingsCheckOption(name, value string, def target.Setting) error {
	if len(def.Options) == 0 {
		return nil
	}
	if slices.Contains(def.Options, value) {
		return nil
	}
	return fmt.Errorf("%s: the config gives %q, which this controller does not offer; it allows %s",
		name, value, strings.Join(def.Options, ", "))
}
