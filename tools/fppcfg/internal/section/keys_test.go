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
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

const (
	keysTestA = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMTMLVB6hEAiTWmtHAM3q6YKMuoS1YCJ1CtsOygXKxG+ test-recovery"
	keysTestB = `cert-authority,principals="operator" ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBOREgPUDNWKEWn5E2/u/7TDcOMSWVRIiAK8YYUc+7JzLeH81Ill5htRkuEXk9X34dxZT44LJGyzN00ejlceFJ80=`
)

func keysTestCfg() *config.Controller {
	return &config.Controller{ManageKeys: true, SSHKeys: []string{keysTestB, keysTestA + "  ", ""}}
}

// keysTestRender shows a state as text, so tests compare one string.
func keysTestRender(st state.State) string { return state.Render(st) }

func TestKeysWant(t *testing.T) {
	got := keysTestRender(NewKeys(keysTestCfg()).Want())
	want := "keys.count = 2\nkeys[0] = " + keysTestB + "\nkeys[1] = " + keysTestA + "\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if st := NewKeys(&config.Controller{SSHKeys: []string{keysTestA}}).Want(); st != nil {
		t.Errorf("unmanaged: %v", st)
	}
}

func TestKeysObserve(t *testing.T) {
	tests := []struct {
		name    string
		api     func(*fake.API)
		want    string
		wantErr string
	}{
		{
			name: "stored copy (recorded, 10.4)",
			api: func(a *fake.API) {
				a.Recorded("fpp10.4", "/api/configfile/authorized_keys")
			},
			want: "keys.count = 2\nkeys[0] = " + keysTestB + "\nkeys[1] = " + keysTestA + "\n",
		},
		{
			name: "blank lines and trailing space",
			api: func(a *fake.API) {
				a.Reply("GET", "/api/configfile/authorized_keys", 200, "\n"+keysTestA+" \n\n")
			},
			want: "keys.count = 1\nkeys[0] = " + keysTestA + "\n",
		},
		{
			name: "no stored copy",
			api: func(a *fake.API) {
				a.Reply("GET", "/api/configfile/authorized_keys", 404, "")
			},
			want: "keys.count = 0\n",
		},
		{
			name: "other failure",
			api: func(a *fake.API) {
				a.Reply("GET", "/api/configfile/authorized_keys", 500, "")
			},
			wantErr: "500",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := fake.NewAPI(t)
			tc.api(api)
			got, err := NewKeys(keysTestCfg()).Observe(context.Background(), &target.Target{API: api})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r := keysTestRender(got); r != tc.want {
				t.Errorf("got\n%s\nwant\n%s", r, tc.want)
			}
		})
	}
}

func TestKeysApply(t *testing.T) {
	tests := []struct {
		name    string
		reply   string
		wantErr string
	}{
		{name: "installed", reply: `{"Status":"OK","Message":""}`},
		{name: "script output before the reply", reply: "done\n" + `{"Status":"OK","Message":""}`},
		{name: "refused in the body", reply: `{"Status":"Error","Message":"Unable to write file"}`, wantErr: "Unable to write file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := fake.NewAPI(t).Reply("POST", "/api/configfile/authorized_keys", 200, tc.reply)
			s := NewKeys(keysTestCfg())
			changes := state.Compare(s.Want(), state.List("keys", nil))
			_, err := s.Apply(context.Background(), &target.Target{API: api}, changes)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %v, want %q", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			want := []fake.Request{{Method: "POST", Path: "/api/configfile/authorized_keys", Body: keysTestB + "\n" + keysTestA + "\n"}}
			if got := api.Writes(); !reflect.DeepEqual(got, want) {
				t.Errorf("writes %q, want %q", got, want)
			}
		})
	}
}
