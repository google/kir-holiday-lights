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
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/fake"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

const (
	passwordsUI = "ui-Secret-1"
	passwordsOS = `os"Secret 2`
)

func passwordsCfg(ui, os string) *config.Controller {
	return &config.Controller{UIPassword: ui, OSPassword: os}
}

func TestPasswordsWant(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Controller
		want     state.State
		rewrites bool
	}{
		{"unmanaged", passwordsCfg("", ""), nil, false},
		{"ui only", passwordsCfg(passwordsUI, ""), state.State{
			{Key: "passwords.ui.enabled", Value: "1"},
		}, true},
		{"both", passwordsCfg(passwordsUI, passwordsOS), state.State{
			{Key: "passwords.ui.enabled", Value: "1"},
			{Key: "passwords.os.enabled", Value: "1"},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewPasswords(tt.cfg)
			if got := s.Want(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Want() = %v, want %v", got, tt.want)
			}
			if got := s.(Rewriter).Rewrites(); got != tt.rewrites {
				t.Errorf("Rewrites() = %v, want %v", got, tt.rewrites)
			}
		})
	}
}

// Only the enable flags are read.
func TestPasswordsObserve(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4", "/api/settings/passwordEnable", "/api/settings/osPasswordEnable")
	got, err := NewPasswords(passwordsCfg(passwordsUI, passwordsOS)).Observe(context.Background(), settingsTarget(api))
	if err != nil {
		t.Fatal(err)
	}
	want := state.State{
		{Key: "passwords.ui.enabled", Value: "0"},
		{Key: "passwords.os.enabled", Value: "0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Observe() = %v, want %v", got, want)
	}
}

func TestPasswordsApply(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.Controller
		have      state.State
		writes    []fake.Request
		passwords []string
	}{
		{
			name: "both new",
			cfg:  passwordsCfg(passwordsUI, passwordsOS),
			have: state.State{
				{Key: "passwords.ui.enabled", Value: "0"},
				{Key: "passwords.os.enabled", Value: "0"},
			},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/password", Body: passwordsUI},
				{Method: "PUT", Path: "/api/settings/passwordEnable", Body: "1"},
				{Method: "PUT", Path: "/api/settings/osPassword", Body: passwordsOS},
				{Method: "PUT", Path: "/api/settings/osPasswordEnable", Body: "1"},
			},
			passwords: []string{passwordsUI},
		},
		{
			name: "already enabled: passwords rewritten, flags left",
			cfg:  passwordsCfg(passwordsUI, passwordsOS),
			have: state.State{
				{Key: "passwords.ui.enabled", Value: "1"},
				{Key: "passwords.os.enabled", Value: "1"},
			},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/password", Body: passwordsUI},
				{Method: "PUT", Path: "/api/settings/osPassword", Body: passwordsOS},
			},
			passwords: []string{passwordsUI},
		},
		{
			name: "os only",
			cfg:  passwordsCfg("", passwordsOS),
			have: state.State{{Key: "passwords.os.enabled", Value: "0"}},
			writes: []fake.Request{
				{Method: "PUT", Path: "/api/settings/osPassword", Body: passwordsOS},
				{Method: "PUT", Path: "/api/settings/osPasswordEnable", Body: "1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := fake.NewAPI(t)
			for _, w := range tt.writes {
				api.Reply(w.Method, w.Path, 200, `{"status":"OK"}`)
			}
			s := NewPasswords(tt.cfg)
			after, err := s.Apply(context.Background(), settingsTarget(api), state.Compare(s.Want(), tt.have))
			if err != nil {
				t.Fatal(err)
			}
			if after != None {
				t.Errorf("After = %v, want none", after)
			}
			if got := api.Writes(); !reflect.DeepEqual(got, tt.writes) {
				t.Errorf("writes = %+v, want %+v", got, tt.writes)
			}
			if got := api.Passwords(); !reflect.DeepEqual(got, tt.passwords) {
				t.Errorf("client passwords: got %d, want %d", len(got), len(tt.passwords))
			}
		})
	}
}

// No error names a password, whatever FPP or the transport says.
func TestPasswordsNotInErrors(t *testing.T) {
	cfg := passwordsCfg(passwordsUI, passwordsOS)
	t.Run("refused write", func(t *testing.T) {
		api := fake.NewAPI(t).Reply("PUT", "/api/settings/password", 500,
			`{"status":"ERROR","message":"could not store `+passwordsUI+`"}`)
		s := NewPasswords(cfg)
		_, err := s.Apply(context.Background(), settingsTarget(api),
			state.Compare(s.Want(), nil))
		if err == nil {
			t.Fatal("Apply() succeeded")
		}
		if strings.Contains(err.Error(), passwordsUI) {
			t.Errorf("error shows the password: %v", err)
		}
	})
	t.Run("transport error", func(t *testing.T) {
		tg := settingsTarget(nil)
		tg.API = &passwordsLeakyAPI{API: fake.NewAPI(t)}
		s := NewPasswords(cfg)
		_, err := s.Apply(context.Background(), tg, state.Compare(s.Want(), nil))
		if err == nil {
			t.Fatal("Apply() succeeded")
		}
		if strings.Contains(err.Error(), passwordsUI) {
			t.Errorf("error shows the password: %v", err)
		}
	})
	t.Run("transport error, os password", func(t *testing.T) {
		tg := settingsTarget(nil)
		tg.API = &passwordsLeakyAPI{API: fake.NewAPI(t)}
		s := NewPasswords(passwordsCfg("", passwordsOS))
		_, err := s.Apply(context.Background(), tg, state.Compare(s.Want(), nil))
		if err == nil {
			t.Fatal("Apply() succeeded")
		}
		if strings.Contains(err.Error(), "Secret 2") {
			t.Errorf("error shows the password: %v", err)
		}
	})
}

// passwordsLeakyAPI fails every request with an error that repeats the body,
// as a careless transport might.
type passwordsLeakyAPI struct{ *fake.API }

func (a *passwordsLeakyAPI) Do(ctx context.Context, method, path string, body io.Reader, contentType string) (*target.Response, error) {
	b, _ := io.ReadAll(body)
	return nil, fmt.Errorf("%s %s: could not send %q", method, path, b)
}
