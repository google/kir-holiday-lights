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
	"strconv"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// audioSection is the volume, which has its own API call. AudioOutput is an
// ordinary setting. Entry: "audio.volume".
type audioSection struct{ cfg *config.Controller }

func NewAudio(cfg *config.Controller) Section { return &audioSection{cfg: cfg} }

func (s *audioSection) Name() string { return "audio" }

const (
	audioKey  = "audio.volume"
	audioPath = "/api/system/volume"
)

func (s *audioSection) Want() state.State {
	if s.cfg.Volume == nil {
		return nil
	}
	return state.State{{Key: audioKey, Value: strconv.Itoa(*s.cfg.Volume)}}
}

func (s *audioSection) Observe(ctx context.Context, t *target.Target) (state.State, error) {
	var reply struct {
		Volume *json.Number `json:"volume"`
	}
	if err := target.GetJSON(ctx, t.API, audioPath, &reply); err != nil {
		return nil, fmt.Errorf("reading the volume: %w", err)
	}
	if reply.Volume == nil {
		return nil, errors.New("reading the volume: no volume in the reply")
	}
	return state.State{{Key: audioKey, Value: reply.Volume.String()}}, nil
}

func (s *audioSection) Apply(ctx context.Context, t *target.Target, changes []state.Change) (After, error) {
	body := struct {
		Volume int `json:"volume"`
	}{*s.cfg.Volume}
	if err := target.PostJSON(ctx, t.API, audioPath, body); err != nil {
		return None, fmt.Errorf("setting the volume: %w", err)
	}
	return None, nil
}
