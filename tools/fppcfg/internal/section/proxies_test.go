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

var proxiesTree = config.Proxy{Host: "192.168.1.155", Description: `Giga "Tree"`}

func TestProxiesWant(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Controller
		want state.State
	}{
		{"unmanaged", config.Controller{Proxies: []config.Proxy{proxiesTree}}, nil},
		{"none", config.Controller{ManageProxies: true}, settingsEntries("proxies.count", "0")},
		{
			"two",
			config.Controller{ManageProxies: true, Proxies: []config.Proxy{proxiesTree, {Host: "arch.local"}}},
			settingsEntries("proxies.count", "2", "proxies[0]", `192.168.1.155 "Giga \"Tree\""`, "proxies[1]", "arch.local"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewProxies(&tt.cfg).Want(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Want() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxiesObserve(t *testing.T) {
	tests := []struct {
		name      string
		reply     string // "" for the 10.4 fixture
		named     []config.Proxy
		want      state.State
		errSubstr string
	}{
		{name: "10.4 empty", want: settingsEntries("proxies.count", "0")},
		{
			name:  "stored",
			reply: `[{"host":"192.168.1.155","description":"Giga \"Tree\"","pending":false},{"host":"arch.local","description":"","pending":false}]`,
			want:  settingsEntries("proxies.count", "2", "proxies[0]", `192.168.1.155 "Giga \"Tree\""`, "proxies[1]", "arch.local"),
		},
		{
			// FPP's own lease proxies show only when the config names the
			// host, and a pending one is not stored at all.
			name: "dhcp",
			reply: `[{"host":"192.168.1.155","description":"","pending":false},` +
				`{"host":"192.168.8.10","description":"","dhcp":true,"pending":false},` +
				`{"host":"192.168.8.11","description":"","dhcp":true,"pending":false},` +
				`{"host":"192.168.8.12","description":"","dhcp":true,"pending":true}]`,
			named: []config.Proxy{{Host: "192.168.8.11"}, {Host: "192.168.8.12"}},
			want:  settingsEntries("proxies.count", "2", "proxies[0]", "192.168.1.155", "proxies[1]", "192.168.8.11"),
		},
		{name: "null", reply: `null`, errSubstr: "no list in the reply"},
		{name: "object", reply: `{"status":"OK"}`, errSubstr: "bad JSON"},
		{name: "strings", reply: `["192.168.1.155"]`, errSubstr: "bad JSON"},
		{name: "no host", reply: `[{"description":"x"}]`, errSubstr: "item 0 has no host"},
		{name: "null item", reply: `[null]`, errSubstr: "item 0 has no host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := fake.NewAPI(t)
			if tt.reply == "" {
				api.Recorded("fpp10.4", "/api/proxies")
			} else {
				api.Reply("GET", "/api/proxies", 200, tt.reply)
			}
			s := NewProxies(&config.Controller{ManageProxies: true, Proxies: tt.named})
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

func TestProxiesApply(t *testing.T) {
	tests := []struct {
		name      string
		items     []config.Proxy
		have      string
		write     fake.Request
		status    int
		reply     string
		errSubstr string
	}{
		{
			name:   "add one",
			items:  []config.Proxy{proxiesTree, {Host: "arch.local"}},
			have:   `[]`,
			write:  fake.Request{Method: "POST", Path: "/api/proxies", Body: `[{"host":"192.168.1.155","description":"Giga \"Tree\""},{"host":"arch.local","description":""}]`},
			status: 200,
			reply:  `[{"host":"192.168.1.155","description":"Giga \"Tree\""},{"host":"arch.local","description":""}]`,
		},
		{
			name:   "remove one",
			items:  []config.Proxy{{Host: "arch.local"}},
			have:   `[{"host":"192.168.1.155","description":"Giga","pending":false},{"host":"arch.local","description":"","pending":false}]`,
			write:  fake.Request{Method: "POST", Path: "/api/proxies", Body: `[{"host":"arch.local","description":""}]`},
			status: 200,
			reply:  `[{"host":"arch.local","description":""}]`,
		},
		{
			name:   "remove all",
			have:   `[{"host":"192.168.1.155","description":"","pending":false}]`,
			write:  fake.Request{Method: "DELETE", Path: "/api/proxies"},
			status: 200,
			reply:  `[]`,
		},
		{
			// FPP's own lease proxy stays when the config's last one goes.
			name: "remove all, lease kept",
			have: `[{"host":"192.168.1.155","description":"","pending":false},` +
				`{"host":"192.168.8.10","description":"","dhcp":true,"pending":false},` +
				`{"host":"192.168.8.11","description":"","dhcp":true,"pending":true}]`,
			write:  fake.Request{Method: "POST", Path: "/api/proxies", Body: `[{"host":"192.168.8.10","description":""}]`},
			status: 200,
			reply:  `[{"host":"192.168.8.10","description":""}]`,
		},
		{
			name:      "refused",
			items:     []config.Proxy{proxiesTree},
			have:      `[]`,
			write:     fake.Request{Method: "POST", Path: "/api/proxies", Body: `[{"host":"192.168.1.155","description":"Giga \"Tree\""}]`},
			status:    400,
			reply:     `{"error":"No valid proxies provided"}`,
			errSubstr: "saving the proxies: POST /api/proxies: HTTP 400: No valid proxies provided",
		},
		{
			// FPP 8.x has no DELETE /api/proxies.
			name:      "remove all on 8.x",
			have:      `[{"host":"192.168.1.155","description":""}]`,
			write:     fake.Request{Method: "DELETE", Path: "/api/proxies"},
			status:    404,
			reply:     `<html>Not found</html>`,
			errSubstr: "removing the proxies: DELETE /api/proxies: HTTP 404",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := fake.NewAPI(t)
			api.Reply("GET", "/api/proxies", 200, tt.have)
			api.Reply(tt.write.Method, tt.write.Path, tt.status, tt.reply)
			tg := settingsTarget(api)
			s := NewProxies(&config.Controller{ManageProxies: true, Proxies: tt.items})
			have, err := s.Observe(context.Background(), tg)
			if err != nil {
				t.Fatal(err)
			}
			after, err := s.Apply(context.Background(), tg, state.Compare(s.Want(), have))
			if got := api.Writes(); !reflect.DeepEqual(got, []fake.Request{tt.write}) {
				t.Errorf("writes = %+v, want %+v", got, []fake.Request{tt.write})
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
			if after != None {
				t.Errorf("After = %v, want none", after)
			}
		})
	}
}
