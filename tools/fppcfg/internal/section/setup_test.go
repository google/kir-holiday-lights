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
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/fake"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
)

// setupConfig matches the fpp10.4 fixtures, with changes applied by name.
func setupConfig(change ...string) *config.Controller {
	cfg := &config.Controller{Setup: []config.Setting{
		{Name: "LegalJurisdiction", Value: "US"},
		{Name: "statsPublish", Value: "Disabled"},
		{Name: "ShareCrashData", Value: "0"},
		{Name: "FetchVendorLogos", Value: "0"},
		{Name: "SendVendorSerial", Value: "0"},
	}}
	for i := 0; i < len(change); i += 2 {
		for j := range cfg.Setup {
			if cfg.Setup[j].Name == change[i] {
				cfg.Setup[j].Value = change[i+1]
			}
		}
	}
	return cfg
}

var setupNames = []string{"LegalJurisdiction", "statsPublish", "ShareCrashData", "FetchVendorLogos", "SendVendorSerial"}

const setupGateUp = `<html><head><meta http-equiv='refresh' content='0;url=initialSetup.php?redirect=index.php'></head></html>`

const setupLimonade404 = `<!DOCTYPE html><html><head><title>Page not found</title></head><body><h1>Page not found</h1></body></html>`

// setup104 is a fake FPP 10.4 with the gate down and consent recorded.
func setup104(t *testing.T) *fake.API {
	api := fake.NewAPI(t)
	for _, n := range setupNames {
		api.Recorded("fpp10.4", "/api/settings/"+n)
	}
	return api.Recorded("fpp10.4", "/api/settings/initialSetup-02", "/api/privacy/consent")
}

// setup8x is a fake FPP 8.4.1, as the bench Pi 4 answered: the jurisdiction
// and the setup step have no definition and answer "[]" until stored, the
// four privacy settings are defined, and there is no consent API. The gate
// is down unless pages says otherwise (the index page's replies, in order).
func setup8x(t *testing.T, consentStatus int, pages ...string) *fake.API {
	api := fake.NewAPI(t)
	api.Recorded("fpp10.4", "/api/settings/statsPublish", "/api/settings/ShareCrashData",
		"/api/settings/FetchVendorLogos", "/api/settings/SendVendorSerial")
	for _, n := range []string{"LegalJurisdiction", "initialSetup-02"} {
		api.Reply("GET", "/api/settings/"+n, 200, "[]")
	}
	api.Reply("GET", "/api/privacy/consent", consentStatus, setupLimonade404)
	if len(pages) == 0 {
		pages = []string{setupGateDown8x}
	}
	for _, p := range pages {
		api.Reply("GET", "/index.php", 200, p)
	}
	return api
}

const setupGateDown8x = "<html><body>FPP 8</body></html>"

func TestSetupWant(t *testing.T) {
	if got := NewSetup(&config.Controller{}).Want(); got != nil {
		t.Errorf("Want() unmanaged = %v, want nil", got)
	}
	want := settingsEntries(
		"setup.LegalJurisdiction", "US",
		"setup.statsPublish", "Disabled",
		"setup.ShareCrashData", "0",
		"setup.FetchVendorLogos", "0",
		"setup.SendVendorSerial", "0",
		"setup.consent", "recorded",
		"setup.gate", "down",
	)
	if got := NewSetup(setupConfig()).Want(); !reflect.DeepEqual(got, want) {
		t.Errorf("Want() = %v, want %v", got, want)
	}
}

func TestSetupObserve(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.Controller
		api       func(*testing.T) *fake.API
		changes   []string // keys
		notes     []string
		errSubstr string
	}{
		{
			name: "10.4 set up",
			cfg:  setupConfig(),
			api: func(t *testing.T) *fake.API {
				return setup104(t).Recorded("fpp10.4", "/index.php")
			},
		},
		{
			name: "10.4 gate up, consent absent",
			cfg:  setupConfig("statsPublish", "Enabled"),
			api: func(t *testing.T) *fake.API {
				api := fake.NewAPI(t)
				for _, n := range setupNames {
					api.Recorded("fpp10.4", "/api/settings/"+n)
				}
				api.Recorded("fpp10.4", "/api/settings/initialSetup-02")
				api.Reply("GET", "/api/privacy/consent", 200, `{"consent":null,"shortfall":"absent","drift":["statsPublish"]}`)
				return api.Reply("GET", "/index.php", 200, setupGateUp)
			},
			changes: []string{"setup.statsPublish", "setup.consent", "setup.gate"},
		},
		{
			name:  "8.x, consent page 404",
			cfg:   setupConfig(),
			api:   func(t *testing.T) *fake.API { return setup8x(t, 404) },
			notes: []string{"LegalJurisdiction: not on this FPP; skipped", "no consent API on this FPP; consent not recorded"},
		},
		{
			name:  "8.x, consent page 200",
			cfg:   setupConfig(),
			api:   func(t *testing.T) *fake.API { return setup8x(t, 200) },
			notes: []string{"LegalJurisdiction: not on this FPP; skipped", "no consent API on this FPP; consent not recorded"},
		},
		{
			name: "jurisdiction not offered",
			cfg:  setupConfig("LegalJurisdiction", "XX"),
			api: func(t *testing.T) *fake.API {
				return fake.NewAPI(t).Recorded("fpp10.4", "/api/settings/LegalJurisdiction")
			},
			errSubstr: `LegalJurisdiction: the config gives "XX"`,
		},
		{
			name: "consent read fails",
			cfg:  setupConfig(),
			api: func(t *testing.T) *fake.API {
				api := fake.NewAPI(t)
				for _, n := range setupNames {
					api.Recorded("fpp10.4", "/api/settings/"+n)
				}
				api.Recorded("fpp10.4", "/api/settings/initialSetup-02")
				return api.Reply("GET", "/api/privacy/consent", 500, "oops")
			},
			errSubstr: "reading the consent record: HTTP 500",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := tt.api(t)
			s := NewSetup(tt.cfg)
			have, err := s.Observe(context.Background(), settingsTarget(api))
			if tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("Observe() error = %v, want it to contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var keys []string
			for _, c := range state.Compare(s.Want(), have) {
				keys = append(keys, c.Key)
			}
			if !slices.Equal(keys, tt.changes) {
				t.Errorf("changes = %q, want %q", keys, tt.changes)
			}
			if n := s.(Notes).Notes(); !slices.Equal(n, tt.notes) {
				t.Errorf("Notes() = %q, want %q", n, tt.notes)
			}
		})
	}
}

func TestSetupConsentValues(t *testing.T) {
	tests := []struct {
		status int
		body   string
		want   string
		ok     bool
	}{
		{200, `{"shortfall":"","drift":[]}`, "recorded", true},
		{200, `{"shortfall":"stale-version","drift":["statsPublish"]}`, "stale-version", true},
		{200, `{"shortfall":"other-device","drift":[]}`, "other-device", true},
		{200, `{"shortfall":"","drift":["statsPublish","ShareCrashData"]}`, "drift: statsPublish, ShareCrashData", true},
		{200, setupLimonade404, "", false},
		{404, setupLimonade404, "", false},
		{200, `{"status":"OK"}`, "", false},
	}
	for _, tt := range tests {
		api := fake.NewAPI(t).Reply("GET", "/api/privacy/consent", tt.status, tt.body)
		got, ok, err := setupReadConsent(context.Background(), api)
		if err != nil || got != tt.want || ok != tt.ok {
			t.Errorf("%d %s: got %q, %v, %v; want %q, %v", tt.status, tt.body, got, ok, err, tt.want, tt.ok)
		}
	}
}

func setupFixedNow(s Section) {
	s.(*setupSection).now = func() time.Time {
		return time.Date(2026, 11, 11, 14, 18, 6, 500, time.FixedZone("PST", -8*3600))
	}
}

func TestSetupApply(t *testing.T) {
	const consentBody = `{"via":"wizard","date":"2026-11-11T22:18:06Z","settings":{"FetchVendorLogos":"0","SendVendorSerial":"0","ShareCrashData":"1","statsPublish":"Disabled"}}`
	tests := []struct {
		name      string
		cfg       *config.Controller
		api       func(*testing.T) *fake.API
		writes    []fake.Request
		after     After
		errSubstr string
	}{
		{
			name: "10.4, a privacy setting changed",
			cfg:  setupConfig("ShareCrashData", "1"),
			api: func(t *testing.T) *fake.API {
				api := setup104(t)
				api.ReplyFile("GET", "/index.php", 200, "fpp10.4/index.php.html")
				api.Reply("POST", "/api/privacy/consent", 200, `{"Status":"OK","consent":{}}`)
				return api
			},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/LegalJurisdiction", Body: "US"},
				{Method: "PUT", Path: "/api/settings/statsPublish", Body: "Disabled"},
				{Method: "PUT", Path: "/api/settings/ShareCrashData", Body: "1"},
				{Method: "PUT", Path: "/api/settings/FetchVendorLogos", Body: "0"},
				{Method: "PUT", Path: "/api/settings/SendVendorSerial", Body: "0"},
				{Method: "POST", Path: "/api/privacy/consent", Body: consentBody},
				{Method: "PUT", Path: "/api/settings/initialSetup-02", Body: "1"},
			},
			after: None,
		},
		{
			name: "10.4, statsPublish asks for a quick restart",
			cfg:  setupConfig("statsPublish", "Enabled", "ShareCrashData", "1"),
			api: func(t *testing.T) *fake.API {
				api := setup104(t)
				api.ReplyFile("GET", "/index.php", 200, "fpp10.4/index.php.html")
				api.Reply("POST", "/api/privacy/consent", 200, `{"Status":"OK"}`)
				return api
			},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/LegalJurisdiction", Body: "US"},
				{Method: "PUT", Path: "/api/settings/statsPublish", Body: "Enabled"},
				{Method: "PUT", Path: "/api/settings/ShareCrashData", Body: "1"},
				{Method: "PUT", Path: "/api/settings/FetchVendorLogos", Body: "0"},
				{Method: "PUT", Path: "/api/settings/SendVendorSerial", Body: "0"},
				{Method: "POST", Path: "/api/privacy/consent", Body: strings.Replace(consentBody, `"statsPublish":"Disabled"`, `"statsPublish":"Enabled"`, 1)},
				{Method: "PUT", Path: "/api/settings/initialSetup-02", Body: "1"},
			},
			after: QuickRestart,
		},
		{
			name: "8.x writes only what exists",
			cfg:  setupConfig("ShareCrashData", "1"),
			api:  func(t *testing.T) *fake.API { return setup8x(t, 404) },
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/statsPublish", Body: "Disabled"},
				{Method: "PUT", Path: "/api/settings/ShareCrashData", Body: "1"},
				{Method: "PUT", Path: "/api/settings/FetchVendorLogos", Body: "0"},
				{Method: "PUT", Path: "/api/settings/SendVendorSerial", Body: "0"},
			},
			after: None,
		},
		{
			// The step has no definition, yet the gate is up until it is
			// stored.
			name: "8.4.1 gate up, step undefined",
			cfg:  setupConfig("ShareCrashData", "1"),
			api: func(t *testing.T) *fake.API {
				return setup8x(t, 404, setupGateUp, setupGateDown8x)
			},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/statsPublish", Body: "Disabled"},
				{Method: "PUT", Path: "/api/settings/ShareCrashData", Body: "1"},
				{Method: "PUT", Path: "/api/settings/FetchVendorLogos", Body: "0"},
				{Method: "PUT", Path: "/api/settings/SendVendorSerial", Body: "0"},
				{Method: "PUT", Path: "/api/settings/initialSetup-02", Body: "1"},
			},
			after: None,
		},
		{
			name: "gate still up",
			cfg:  setupConfig("ShareCrashData", "1"),
			api: func(t *testing.T) *fake.API {
				api := setup104(t)
				api.Reply("GET", "/index.php", 200, setupGateUp)
				api.Reply("POST", "/api/privacy/consent", 200, `{"Status":"OK"}`)
				return api
			},
			errSubstr: "the setup gate is still up",
		},
		{
			name: "consent refused in a 200",
			cfg:  setupConfig("ShareCrashData", "1"),
			api: func(t *testing.T) *fake.API {
				api := setup104(t).Recorded("fpp10.4", "/index.php")
				api.Reply("POST", "/api/privacy/consent", 200, `{"Status":"Error","Message":"unknown consent origin"}`)
				return api
			},
			errSubstr: "recording consent: POST /api/privacy/consent: HTTP 200: unknown consent origin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := tt.api(t)
			for _, n := range append(setupNames, "initialSetup-02") {
				api.Reply("PUT", "/api/settings/"+n, 200, `{"status":"OK"}`)
			}
			tg := settingsTarget(api)
			s := NewSetup(tt.cfg)
			setupFixedNow(s)
			have, err := s.Observe(context.Background(), tg)
			if err != nil {
				t.Fatal(err)
			}
			changes := state.Compare(s.Want(), have)
			if len(changes) == 0 {
				t.Fatal("no changes to apply")
			}
			after, err := s.Apply(context.Background(), tg, changes)
			if tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("Apply() error = %v, want it to contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if after != tt.after {
				t.Errorf("After = %v, want %v", after, tt.after)
			}
			if got := api.Writes(); !reflect.DeepEqual(got, tt.writes) {
				t.Errorf("writes =\n%+v\nwant\n%+v", got, tt.writes)
			}
			// The gate is read again at the end.
			if r := api.Requests(); r[len(r)-1].Path != "/index.php" {
				t.Errorf("last request = %s %s, want the gate read", r[len(r)-1].Method, r[len(r)-1].Path)
			}
		})
	}
}
