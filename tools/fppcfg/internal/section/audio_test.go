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
	"strings"
	"testing"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/fake"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
)

func TestAudio(t *testing.T) {
	if got := NewAudio(&config.Controller{}).Want(); got != nil {
		t.Errorf("Want() unmanaged = %v, want nil", got)
	}
	vol := 50
	tests := []struct {
		name      string
		reply     string
		have      state.State
		errSubstr string
	}{
		{name: "recorded", have: settingsEntries("audio.volume", "70")},
		{name: "no volume", reply: `{"status":"OK"}`, errSubstr: "no volume in the reply"},
		{name: "refused", reply: `{"status":"ERROR","message":"no fppd"}`, errSubstr: "reading the volume: GET /api/system/volume: HTTP 200: no fppd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := fake.NewAPI(t)
			if tt.reply == "" {
				api.Recorded("fpp10.4", "/api/system/volume")
			} else {
				api.Reply("GET", "/api/system/volume", 200, tt.reply)
			}
			api.Reply("POST", "/api/system/volume", 200, `{"status":"OK","volume":50}`)
			tg := settingsTarget(api)
			s := NewAudio(&config.Controller{Volume: &vol})
			want := s.Want()
			if !reflect.DeepEqual(want, settingsEntries("audio.volume", "50")) {
				t.Errorf("Want() = %v", want)
			}
			have, err := s.Observe(context.Background(), tg)
			if tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("Observe() error = %v, want it to contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(have, tt.have) {
				t.Errorf("Observe() = %v, want %v", have, tt.have)
			}
			after, err := s.Apply(context.Background(), tg, state.Compare(want, have))
			if err != nil {
				t.Fatal(err)
			}
			if after != None {
				t.Errorf("After = %v, want none", after)
			}
			writes := []fake.Request{{Method: "POST", Path: "/api/system/volume", Body: `{"volume":50}`}}
			if got := api.Writes(); !reflect.DeepEqual(got, writes) {
				t.Errorf("writes = %+v, want %+v", got, writes)
			}
		})
	}
}
