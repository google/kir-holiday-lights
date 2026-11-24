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

package target_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/fake"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

var ctx = context.Background()

func TestGetJSON(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4", "/api/fppd/version")
	var v struct{ Version string }
	if err := target.GetJSON(ctx, api, "/api/fppd/version", &v); err != nil {
		t.Fatal(err)
	}
	if v.Version != "10.4" {
		t.Errorf("version %q", v.Version)
	}
}

func TestGetJSONErrors(t *testing.T) {
	api := fake.NewAPI(t).
		Reply("GET", "/api/missing", 404, "<html>not found</html>").
		Reply("GET", "/api/html", 200, "<html>")
	var v any
	err := target.GetJSON(ctx, api, "/api/missing", &v)
	var se *target.StatusError
	if !errors.As(err, &se) || se.Status != 404 || strings.Contains(err.Error(), "html") {
		t.Errorf("404: %v", err)
	}
	if err := target.GetJSON(ctx, api, "/api/html", &v); err == nil || !strings.Contains(err.Error(), "/api/html") {
		t.Errorf("bad JSON: %v", err)
	}
}

func TestPostJSONStatusInBody(t *testing.T) {
	api := fake.NewAPI(t).
		Reply("POST", "/api/upper", 200, `{"Status":"Error","Message":"Invalid regime"}`).
		Reply("POST", "/api/lower", 200, `{"status":"ERROR","message":"No event 3"}`).
		Reply("POST", "/api/ok", 200, `{"status":"OK","n":2}`)
	for path, want := range map[string]string{"/api/upper": "Invalid regime", "/api/lower": "No event 3"} {
		err := target.PostJSON(ctx, api, path, map[string]string{"a": "b"})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", path, err)
		}
	}
	if err := target.PostJSON(ctx, api, "/api/ok", map[string]int{"x": 1}); err != nil {
		t.Errorf("ok: %v", err)
	}
	if got := api.Writes()[2].Body; got != `{"x":1}` {
		t.Errorf("sent %q", got)
	}
}

func TestGetSetting(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4",
		"/api/settings/fppMode",
		"/api/settings/HostName",
		"/api/settings/uiLevel",
		"/api/settings/MultiSyncEnabled",
		"/api/settings/password",
		"/api/settings/restartFlag",
		"/api/settings/initialSetup-02",
		"/api/settings/TimeZone",
		"/api/settings/MultiSyncMulticast",
		"/api/settings/MultiSyncBroadcast",
		"/api/settings/MultiSyncUnicast",
	)
	for _, tc := range []struct {
		name    string
		defined bool
		value   string
		restart int
		reboot  int
	}{
		{"fppMode", true, "player", 2, 0},
		{"HostName", true, "fpp-bench", 0, 1},
		{"uiLevel", true, "0", 0, 0},          // a number
		{"MultiSyncEnabled", true, "0", 2, 0}, // never set: an unchecked checkbox
		{"password", true, "", 0, 0},          // 10.4 withholds it
		{"restartFlag", false, "0", 0, 0},     // unknown name: only a value
		{"initialSetup-02", true, "1", 0, 0},  // answers as initialSetup-01
		{"TimeZone", true, "Europe/London", 2, 0},
		// The show player and a remote, 10.4 on a Pi 5 and a BeagleBone.
		{"MultiSyncMulticast", true, "1", 0, 0}, // stored
		{"MultiSyncBroadcast", true, "0", 0, 0}, // never set, no default
		{"MultiSyncUnicast", true, "1", 0, 0},   // never set: the default, as a number
	} {
		s, err := target.GetSetting(ctx, api, tc.name)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if s.Defined != tc.defined || s.Value != tc.value ||
			s.Restart != tc.restart || s.Reboot != tc.reboot {
			t.Errorf("%s: got %+v", tc.name, s)
		}
	}
}

// FPP 8.4.1 answers "[]" for a name it neither defines nor has stored.
func TestGetSettingUnknownOn8(t *testing.T) {
	api := fake.NewAPI(t).
		Reply("GET", "/api/settings/LegalJurisdiction", 200, "[]").
		Reply("GET", "/api/settings/uiLevel", 200, `["x"]`)
	s, err := target.GetSetting(ctx, api, "LegalJurisdiction")
	if err != nil || s.Defined || s.Value != "" {
		t.Errorf("got %+v, %v", s, err)
	}
	if _, err := target.GetSetting(ctx, api, "uiLevel"); err == nil {
		t.Error("a non-empty array was accepted")
	}
}

func TestGetSettingOptions(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4",
		"/api/settings/fppMode", "/api/settings/uiLevel", "/api/settings/TimeZone")
	mode, _ := target.GetSetting(ctx, api, "fppMode")
	if !slices.Contains(mode.Options, "remote") {
		t.Errorf("fppMode options %v", mode.Options)
	}
	ui, _ := target.GetSetting(ctx, api, "uiLevel")
	if !slices.Contains(ui.Options, "1") {
		t.Errorf("uiLevel options %v", ui.Options)
	}
	// TimeZone's options are a list, not a map.
	tz, _ := target.GetSetting(ctx, api, "TimeZone")
	if !slices.Contains(tz.Options, "Europe/London") {
		t.Errorf("TimeZone options: %d entries", len(tz.Options))
	}
}

func TestPutSetting(t *testing.T) {
	api := fake.NewAPI(t).
		Reply("PUT", "/api/settings/HostName", 200, `{"status":"OK"}`).
		Reply("PUT", "/api/settings/uiLevel", 400, `{"status":"ERROR","message":"Invalid value 1 for setting, expected 0-3"}`)
	if err := target.PutSetting(ctx, api, "HostName", "fpp-bench"); err != nil {
		t.Fatal(err)
	}
	err := target.PutSetting(ctx, api, "uiLevel", "1")
	if err == nil || !strings.Contains(err.Error(), "Invalid value 1 for setting, expected 0-3") || !strings.Contains(err.Error(), "400") {
		t.Errorf("refused write: %v", err)
	}
	if w := api.Writes(); len(w) != 2 || w[0].Body != "fpp-bench" {
		t.Errorf("writes %+v", w)
	}
}

func TestWaitFor(t *testing.T) {
	n := 0
	err := target.WaitFor(ctx, func(context.Context) bool { n++; return n == 3 }, time.Millisecond)
	if err != nil || n != 3 {
		t.Errorf("err %v after %d", err, n)
	}
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err := target.WaitFor(short, func(context.Context) bool { return false }, time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("timeout: %v", err)
	}
}
