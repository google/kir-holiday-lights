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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// processorsSection is the output processors list, written whole. Entries:
// "processors.count", then "processors[i]" as canonical JSON (keys sorted,
// no spaces), the same form the config package gives.
type processorsSection struct{ cfg *config.Controller }

func NewProcessors(cfg *config.Controller) Section { return &processorsSection{cfg: cfg} }

func (s *processorsSection) Name() string { return "processors" }

const processorsPath = "/api/channel/output/processors"

func (s *processorsSection) Want() state.State {
	if !s.cfg.ManageProcessors {
		return nil
	}
	return state.List("processors", s.cfg.OutputProcessors)
}

func (s *processorsSection) Observe(ctx context.Context, t *target.Target) (state.State, error) {
	var reply struct {
		OutputProcessors *[]json.RawMessage `json:"outputProcessors"`
	}
	if err := target.GetJSON(ctx, t.API, processorsPath, &reply); err != nil {
		return nil, fmt.Errorf("reading the output processors: %w", err)
	}
	if reply.OutputProcessors == nil {
		return nil, errors.New("reading the output processors: no outputProcessors in the reply")
	}
	items := make([]string, 0, len(*reply.OutputProcessors))
	for i, raw := range *reply.OutputProcessors {
		c, err := config.CanonicalJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("reading the output processors: item %d: %w", i, err)
		}
		items = append(items, c)
	}
	return state.List("processors", items), nil
}

// Apply replaces the whole list. A quick restart follows any change: 8.4.2
// needs one, 10.4 does not, and nothing tells the two apart.
func (s *processorsSection) Apply(ctx context.Context, t *target.Target, changes []state.Change) (After, error) {
	items := make([]json.RawMessage, 0, len(s.cfg.OutputProcessors))
	for _, item := range s.cfg.OutputProcessors {
		items = append(items, json.RawMessage(item))
	}
	body := struct {
		OutputProcessors []json.RawMessage `json:"outputProcessors"`
	}{items}
	if err := target.PostJSON(ctx, t.API, processorsPath, body); err != nil {
		return None, fmt.Errorf("saving the output processors: %w", err)
	}
	return QuickRestart, nil
}
