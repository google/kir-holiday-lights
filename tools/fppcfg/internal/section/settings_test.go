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
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/fake"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// settingsTarget is a target over fakes, for the tests of settings, setup,
// audio, processors and passwords.
func settingsTarget(api *fake.API) *target.Target {
	return &target.Target{API: api, Address: "10.0.0.5"}
}

// settingsUndefined is FPP 10.4's reply for a name it has no definition for.
const settingsUndefined = `{"value":""}`

func settingsEntries(pairs ...string) state.State {
	var out state.State
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, state.Entry{Key: pairs[i], Value: pairs[i+1]})
	}
	return out
}

func TestSettingsWant(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Controller
		want state.State
	}{
		{"unmanaged", config.Controller{}, nil},
		{"in config order", config.Controller{Settings: []config.Setting{
			{Name: "HostDescription", Value: "Bench"},
			{Name: "fppMode", Value: "remote"},
		}}, settingsEntries("settings.HostDescription", "Bench", "settings.fppMode", "remote")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSettings(&tt.cfg).Want()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Want() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSettingsObserve(t *testing.T) {
	tests := []struct {
		name      string
		settings  []config.Setting
		replies   func(*fake.API)
		want      state.State
		notes     []string
		errSubstr string
	}{
		{
			name: "recorded values",
			settings: []config.Setting{
				{Name: "fppMode", Value: "remote"},
				{Name: "blankBetweenSequences", Value: "1"},
				{Name: "HostName", Value: "fpp-bench"},
			},
			replies: func(a *fake.API) {
				a.Recorded("fpp10.4", "/api/settings/fppMode", "/api/settings/blankBetweenSequences", "/api/settings/HostName")
			},
			// A checkbox with no value and no default reads as unchecked.
			want: settingsEntries("settings.fppMode", "player", "settings.blankBetweenSequences", "0", "settings.HostName", "fpp-bench"),
		},
		{
			name: "not on this FPP",
			settings: []config.Setting{
				{Name: "fppMode", Value: "player"},
				{Name: "NewThing", Value: "7"},
			},
			replies: func(a *fake.API) {
				a.Recorded("fpp10.4", "/api/settings/fppMode")
				a.Reply("GET", "/api/settings/NewThing", 200, settingsUndefined)
			},
			want:  settingsEntries("settings.fppMode", "player", "settings.NewThing", "7"),
			notes: []string{"NewThing: not on this FPP; skipped"},
		},
		{
			name:     "value not among the options",
			settings: []config.Setting{{Name: "fppMode", Value: "bridge"}},
			replies: func(a *fake.API) {
				a.Recorded("fpp10.4", "/api/settings/fppMode")
			},
			errSubstr: `fppMode: the config gives "bridge", which this controller does not offer; it allows player, remote`,
		},
		{
			name:     "read fails",
			settings: []config.Setting{{Name: "fppMode", Value: "player"}},
			replies: func(a *fake.API) {
				a.Reply("GET", "/api/settings/fppMode", 500, `{"status":"ERROR","message":"boom"}`)
			},
			errSubstr: "reading setting fppMode: GET /api/settings/fppMode: HTTP 500: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := fake.NewAPI(t)
			tt.replies(api)
			s := NewSettings(&config.Controller{Settings: tt.settings})
			got, err := s.Observe(context.Background(), settingsTarget(api))
			if tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("Observe() error = %v, want it to contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Observe() = %v, want %v", got, tt.want)
			}
			if n := s.(Notes).Notes(); !slices.Equal(n, tt.notes) {
				t.Errorf("Notes() = %q, want %q", n, tt.notes)
			}
		})
	}
}

func TestSettingsApply(t *testing.T) {
	tests := []struct {
		name     string
		settings []config.Setting
		recorded []string
		extra    func(*fake.API)
		writes   []fake.Request
		after    After
	}{
		{
			name: "quick restart",
			settings: []config.Setting{
				{Name: "HostDescription", Value: "Bench"},
				{Name: "fppMode", Value: "remote"},
				{Name: "blankBetweenSequences", Value: "0"}, // unchanged: unset reads as "0"
			},
			recorded: []string{"HostDescription", "fppMode", "blankBetweenSequences"},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/HostDescription", Body: "Bench"},
				{Method: "PUT", Path: "/api/settings/fppMode", Body: "remote"},
			},
			after: QuickRestart,
		},
		{
			name: "reboot outranks restart",
			settings: []config.Setting{
				{Name: "fppMode", Value: "remote"},
				{Name: "AudioOutput", Value: "vc4hdmi0"},
			},
			recorded: []string{"fppMode", "AudioOutput"},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/fppMode", Body: "remote"},
				{Method: "PUT", Path: "/api/settings/AudioOutput", Body: "vc4hdmi0"},
			},
			after: Reboot,
		},
		{
			// The show player's multicast-only MultiSync, against a remote's
			// untouched values. Unicast defaults on, so off must be written;
			// broadcast unset already reads as off.
			name: "multisync transport",
			settings: []config.Setting{
				{Name: "MultiSyncMulticast", Value: "1"},
				{Name: "MultiSyncBroadcast", Value: "0"},
				{Name: "MultiSyncUnicast", Value: "0"},
			},
			recorded: []string{"MultiSyncBroadcast", "MultiSyncUnicast"},
			extra: func(a *fake.API) {
				a.Reply("GET", "/api/settings/MultiSyncMulticast", 200,
					`{"name":"MultiSyncMulticast","type":"checkbox","fppModes":["player"]}`)
			},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/MultiSyncMulticast", Body: "1"},
				{Method: "PUT", Path: "/api/settings/MultiSyncUnicast", Body: "0"},
			},
			after: None,
		},
		{
			name: "undefined is not written",
			settings: []config.Setting{
				{Name: "NewThing", Value: "7"},
				{Name: "blankBetweenSequences", Value: "1"},
			},
			recorded: []string{"blankBetweenSequences"},
			extra: func(a *fake.API) {
				a.Reply("GET", "/api/settings/NewThing", 200, settingsUndefined)
			},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/blankBetweenSequences", Body: "1"},
			},
			after: None,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := fake.NewAPI(t)
			for _, n := range tt.recorded {
				api.Recorded("fpp10.4", "/api/settings/"+n)
			}
			if tt.extra != nil {
				tt.extra(api)
			}
			for _, w := range tt.writes {
				api.Reply(w.Method, w.Path, 200, `{"status":"OK"}`)
			}
			tg := settingsTarget(api)
			s := NewSettings(&config.Controller{Settings: tt.settings})
			have, err := s.Observe(context.Background(), tg)
			if err != nil {
				t.Fatal(err)
			}
			after, err := s.Apply(context.Background(), tg, state.Compare(s.Want(), have))
			if err != nil {
				t.Fatal(err)
			}
			if after != tt.after {
				t.Errorf("After = %v, want %v", after, tt.after)
			}
			if got := api.Writes(); !reflect.DeepEqual(got, tt.writes) {
				t.Errorf("writes = %+v, want %+v", got, tt.writes)
			}
		})
	}
}

func TestSettingsApplyRefused(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4", "/api/settings/HostName")
	api.Reply("PUT", "/api/settings/HostName", 400, `{"status":"ERROR","message":"invalid host name"}`)
	tg := settingsTarget(api)
	s := NewSettings(&config.Controller{Settings: []config.Setting{{Name: "HostName", Value: "bench-2"}}})
	have, err := s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(context.Background(), tg, state.Compare(s.Want(), have))
	if err == nil || !strings.Contains(err.Error(), "writing setting HostName") {
		t.Errorf("Apply() error = %v, want a refused write of HostName", err)
	}
}

func TestSettingsUnset(t *testing.T) {
	managed := config.ManagedSettings()
	if len(managed) < 2 {
		t.Skip("schema lists fewer than two settings")
	}
	api := fake.NewAPI(t)
	var want state.State
	inConfig := managed[0]
	for _, name := range managed[1:] {
		path := "/api/settings/" + name
		file := filepath.Join(fake.Testdata(), "fpp10.4", "api_settings_"+name+".json")
		if _, err := os.Stat(file); err != nil {
			api.Reply("GET", path, 200, settingsUndefined)
			continue
		}
		api.Recorded("fpp10.4", path)
		def, err := target.GetSetting(context.Background(), fake.NewAPI(t).Recorded("fpp10.4", path), name)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, state.Entry{Key: "settings." + name, Value: def.Value})
	}
	if len(want) == 0 {
		t.Fatal("no managed setting has a 10.4 fixture")
	}
	s := NewSettings(&config.Controller{Settings: []config.Setting{{Name: inConfig, Value: "x"}}})
	got, err := s.(Unsetter).Unset(context.Background(), settingsTarget(api))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Unset() = %v, want %v", got, want)
	}
	for _, r := range api.Requests() {
		if r.Path == "/api/settings/"+inConfig {
			t.Errorf("Unset read %s, which the config sets", inConfig)
		}
	}
}
