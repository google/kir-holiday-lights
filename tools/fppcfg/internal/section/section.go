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

// Package section holds the parts of a controller fppcfg manages, one file
// per section. Each turns the config into wanted state, reads the
// controller into observed state, and applies the changes push printed.
package section

import (
	"context"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// After is what a section's changes need before they take effect, in the
// tool's own order of severity. (FPP's own "restart" numbers run the other
// way: 1 is full, 2 is quick.)
type After int

const (
	None After = iota
	QuickRestart
	Restart
	Reboot
)

func (a After) String() string {
	return [...]string{"none", "quick restart", "restart", "reboot"}[a]
}

// Section is one managed part of a controller.
type Section interface {
	Name() string
	// Want is the wanted state, from the config alone. It is empty when the
	// config does not manage this section.
	Want() state.State
	// Observe reads the controller. It may remember what it learned (setting
	// definitions, stored interface configs) for Apply in the same run; Apply
	// must not read again to decide what to do. An error stops the run.
	Observe(ctx context.Context, t *target.Target) (state.State, error)
	// Apply makes the changes, which are this section's share of what push
	// printed, and says what they need afterwards.
	Apply(ctx context.Context, t *target.Target, changes []state.Change) (After, error)
}

// Unsetter is implemented by sections with managed keys the config can leave
// unset. diff --full lists them with their current values.
type Unsetter interface {
	Unset(ctx context.Context, t *target.Target) (state.State, error)
}

// Rewriter is implemented by sections that write something the tool cannot
// read back (passwords on FPP 10.4). diff leaves it out. When Rewrites
// returns true, push calls Apply on every run, with whatever changes diff
// did show, possibly none.
type Rewriter interface {
	Rewrites() bool
}

// Notes is implemented by sections that skip something this controller's FPP
// lacks. Observe fills them; the engine prints them.
type Notes interface {
	Notes() []string
}

// All returns the sections for cfg in the order push runs them. Network is
// last; the engine applies it after an fppd restart and before a reboot.
func All(cfg *config.Controller) []Section {
	return []Section{
		NewKeys(cfg),
		NewSettings(cfg),
		NewSetup(cfg),
		NewAudio(cfg),
		NewProcessors(cfg),
		NewProxies(cfg),
		NewPasswords(cfg),
		NewNetwork(cfg),
	}
}
