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

const processorsRemap = `{"active":1,"count":3,"description":"swap","destination":11,"loops":1,"reverse":0,"source":1,"type":"Remap"}`

func TestProcessorsWant(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Controller
		want state.State
	}{
		{"unmanaged", config.Controller{OutputProcessors: []string{processorsRemap}}, nil},
		{"none", config.Controller{ManageProcessors: true}, settingsEntries("processors.count", "0")},
		{"one", config.Controller{ManageProcessors: true, OutputProcessors: []string{processorsRemap}},
			settingsEntries("processors.count", "1", "processors[0]", processorsRemap)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewProcessors(&tt.cfg).Want(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Want() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcessorsObserve(t *testing.T) {
	tests := []struct {
		name      string
		reply     string // "" for the 10.4 fixture
		want      state.State
		errSubstr string
	}{
		{name: "10.4 empty", want: settingsEntries("processors.count", "0")},
		{
			name: "canonical form",
			// Keys out of order, spaces, and a number FPP writes plainly.
			reply: `{"status":"OK","outputProcessors":[ {"type": "Remap", "source": 1, "destination": 11, "count": 3, "loops": 1, "reverse": 0, "active": 1, "description": "swap"}, {"type":"Brightness","brightness":0.5,"gamma":1.0,"start":1,"count":30,"active":1,"description":"a<b"} ]}`,
			want: settingsEntries(
				"processors.count", "2",
				"processors[0]", processorsRemap,
				"processors[1]", `{"active":1,"brightness":0.5,"count":30,"description":"a<b","gamma":1.0,"start":1,"type":"Brightness"}`,
			),
		},
		{name: "no list", reply: `{"status":"OK"}`, errSubstr: "no outputProcessors in the reply"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := fake.NewAPI(t)
			if tt.reply == "" {
				api.Recorded("fpp10.4", "/api/channel/output/processors")
			} else {
				api.Reply("GET", "/api/channel/output/processors", 200, tt.reply)
			}
			s := NewProcessors(&config.Controller{ManageProcessors: true})
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
		})
	}
}

func TestProcessorsApply(t *testing.T) {
	tests := []struct {
		name      string
		items     []string
		have      string
		reply     string
		body      string
		errSubstr string
	}{
		{
			name:  "add one",
			items: []string{processorsRemap},
			have:  `{"status":"OK","outputProcessors":[]}`,
			reply: `{"status":"OK","outputProcessors":[` + processorsRemap + `]}`,
			body:  `{"outputProcessors":[` + processorsRemap + `]}`,
		},
		{
			name:  "remove all",
			have:  `{"status":"OK","outputProcessors":[` + processorsRemap + `]}`,
			reply: `{"status":"OK","outputProcessors":[]}`,
			body:  `{"outputProcessors":[]}`,
		},
		{
			name:      "refused",
			items:     []string{processorsRemap},
			have:      `{"status":"OK","outputProcessors":[]}`,
			reply:     `{"status":"ERROR","message":"cannot write"}`,
			body:      `{"outputProcessors":[` + processorsRemap + `]}`,
			errSubstr: "saving the output processors: POST /api/channel/output/processors: HTTP 200: cannot write",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := fake.NewAPI(t)
			api.Reply("GET", "/api/channel/output/processors", 200, tt.have)
			api.Reply("POST", "/api/channel/output/processors", 200, tt.reply)
			tg := settingsTarget(api)
			s := NewProcessors(&config.Controller{ManageProcessors: true, OutputProcessors: tt.items})
			have, err := s.Observe(context.Background(), tg)
			if err != nil {
				t.Fatal(err)
			}
			after, err := s.Apply(context.Background(), tg, state.Compare(s.Want(), have))
			writes := []fake.Request{{Method: "POST", Path: "/api/channel/output/processors", Body: tt.body}}
			if got := api.Writes(); !reflect.DeepEqual(got, writes) {
				t.Errorf("writes = %+v, want %+v", got, writes)
			}
			if tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("Apply() error = %v, want it to contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if after != QuickRestart {
				t.Errorf("After = %v, want quick restart", after)
			}
		})
	}
}
