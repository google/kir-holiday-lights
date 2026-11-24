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
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// setupSection is the setup gate: LegalJurisdiction and the four privacy
// settings, the consent record, and initialSetup-02. Newer FPP sends every
// UI page to initialSetup.php until all of them are in place.
//
// Entries: "setup.<Name>" for each setting, "setup.consent" ("recorded",
// or what is missing) and "setup.gate" ("up" or "down").
type setupSection struct {
	cfg *config.Controller
	// From Observe.
	defs       map[string]target.Setting
	stepDef    bool // initialSetup-02 is defined (not on 8.4.x, which still has the gate)
	consentAPI bool
	notes      []string
	// now makes the consent date; tests replace it.
	now func() time.Time
}

func NewSetup(cfg *config.Controller) Section {
	return &setupSection{cfg: cfg, now: time.Now}
}

func (s *setupSection) Name() string { return "setup" }

const (
	setupConsentKey = "setup.consent"
	setupGateKey    = "setup.gate"
	setupRecorded   = "recorded"
	setupStep       = "initialSetup-02"
	setupConsent    = "/api/privacy/consent"
	setupIndex      = "/index.php"
)

// setupConsentSettings are the settings the consent record holds.
var setupConsentSettings = []string{"statsPublish", "FetchVendorLogos", "SendVendorSerial", "ShareCrashData"}

func setupKey(name string) string { return "setup." + name }

func (s *setupSection) Want() state.State {
	if len(s.cfg.Setup) == 0 {
		return nil
	}
	var out state.State
	for _, st := range s.cfg.Setup {
		out = append(out, state.Entry{Key: setupKey(st.Name), Value: st.Value})
	}
	return append(out,
		state.Entry{Key: setupConsentKey, Value: setupRecorded},
		state.Entry{Key: setupGateKey, Value: "down"},
	)
}

// Observe reads the five settings, probes for the consent API and reads the
// gate. A setting or consent API this FPP lacks is reported with the wanted
// value and a note, the same way as in the settings section (see
// fppsettings.go).
func (s *setupSection) Observe(ctx context.Context, t *target.Target) (state.State, error) {
	out, defs, notes, err := settingsObserve(ctx, t.API, s.cfg.Setup, setupKey)
	if err != nil {
		return nil, err
	}
	s.defs, s.notes = defs, notes

	step, err := target.GetSetting(ctx, t.API, setupStep)
	if err != nil {
		return nil, fmt.Errorf("reading setting %s: %w", setupStep, err)
	}
	s.stepDef = step.Defined

	consent, ok, err := setupReadConsent(ctx, t.API)
	if err != nil {
		return nil, err
	}
	s.consentAPI = ok
	if !ok {
		s.notes = append(s.notes, "no consent API on this FPP; consent not recorded")
		consent = setupRecorded
	}
	out = append(out, state.Entry{Key: setupConsentKey, Value: consent})

	gate, err := setupReadGate(ctx, t.API)
	if err != nil {
		return nil, err
	}
	return append(out, state.Entry{Key: setupGateKey, Value: gate}), nil
}

func (s *setupSection) Notes() []string { return s.notes }

// setupReadConsent probes for the consent API and reads the record's state:
// "recorded", the shortfall, or "drift: <settings>". ok is false when this
// FPP has no consent API (8.x answers with an HTML error page, as 404 or
// 200).
func setupReadConsent(ctx context.Context, api target.API) (consent string, ok bool, err error) {
	resp, err := api.Do(ctx, http.MethodGet, setupConsent, nil, "")
	if err != nil {
		return "", false, fmt.Errorf("reading the consent record: %w", err)
	}
	var reply struct {
		Shortfall *string  `json:"shortfall"`
		Drift     []string `json:"drift"`
	}
	isJSON := json.Unmarshal(resp.Body, &reply) == nil
	switch {
	case resp.OK() && isJSON && reply.Shortfall != nil:
	case resp.OK() || resp.Status == http.StatusNotFound:
		return "", false, nil
	default:
		// A server error is a failed read, not an answer about the API.
		return "", false, fmt.Errorf("reading the consent record: HTTP %d", resp.Status)
	}
	switch {
	case *reply.Shortfall != "":
		return *reply.Shortfall, true, nil
	case len(reply.Drift) > 0:
		return "drift: " + strings.Join(reply.Drift, ", "), true, nil
	}
	return setupRecorded, true, nil
}

// setupReadGate reads whether the UI still sends pages to initialSetup.php.
// The gate is a meta refresh in a 200 page, not an HTTP redirect.
func setupReadGate(ctx context.Context, api target.API) (string, error) {
	resp, err := api.Do(ctx, http.MethodGet, setupIndex, nil, "")
	if err != nil {
		return "", fmt.Errorf("reading the setup gate: %w", err)
	}
	if !resp.OK() {
		return "", fmt.Errorf("reading the setup gate: GET %s: HTTP %d", setupIndex, resp.Status)
	}
	if strings.Contains(string(resp.Body), "url=initialSetup.php") {
		return "up", nil
	}
	return "down", nil
}

// Apply stores all five settings this FPP has, even those that already read
// as wanted (a default reads the same as a stored value, and the gate needs
// them stored), records consent with the config's values, marks the setup
// step done, and checks that the gate is down.
func (s *setupSection) Apply(ctx context.Context, t *target.Target, changes []state.Change) (After, error) {
	changed := state.Changed(changes)
	after := None
	for _, st := range s.cfg.Setup {
		def := s.defs[st.Name]
		if !def.Defined {
			continue
		}
		if err := target.PutSetting(ctx, t.API, st.Name, st.Value); err != nil {
			return after, fmt.Errorf("writing setting %s: %w", st.Name, err)
		}
		if changed[setupKey(st.Name)] {
			after = max(after, settingsAfter(def))
		}
	}

	if s.consentAPI {
		values := make(map[string]string, len(setupConsentSettings))
		for _, st := range s.cfg.Setup {
			if slices.Contains(setupConsentSettings, st.Name) {
				values[st.Name] = st.Value
			}
		}
		body := struct {
			Via      string            `json:"via"`
			Date     string            `json:"date"`
			Settings map[string]string `json:"settings"`
		}{"wizard", s.now().UTC().Format("2006-01-02T15:04:05Z"), values}
		if err := target.PostJSON(ctx, t.API, setupConsent, body); err != nil {
			return after, fmt.Errorf("recording consent: %w", err)
		}
	}

	// 8.4.x has the gate but no definition for its setting: the gate being
	// up is what says the write is wanted there.
	if s.stepDef || changed[setupGateKey] {
		if err := target.PutSetting(ctx, t.API, setupStep, "1"); err != nil {
			return after, fmt.Errorf("writing setting %s: %w", setupStep, err)
		}
	}

	gate, err := setupReadGate(ctx, t.API)
	if err != nil {
		return after, err
	}
	if gate != "down" {
		return after, errors.New("the setup gate is still up after storing the setup settings and recording consent")
	}
	return after, nil
}
