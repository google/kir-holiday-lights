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

package state

import (
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	want := State{
		{Key: "a", Value: "1"},
		{Key: "b", Value: "2"},
		{Key: "pw.enabled", Value: "1"},
		{Key: "keys[0]", Value: "k0"},
	}
	have := State{
		{Key: "a", Value: "1"},
		{Key: "pw.enabled", Value: "0"},
		{Key: "keys[0]", Value: "k0"},
		{Key: "keys[1]", Value: "k1"},
	}
	got := Compare(want, have)
	var keys []string
	for _, c := range got {
		keys = append(keys, c.Key)
	}
	if strings.Join(keys, " ") != "b pw.enabled keys[1]" {
		t.Fatalf("changes = %v", keys)
	}
	if got[0].Have != nil || got[2].Want != nil {
		t.Errorf("missing sides not nil: %+v", got)
	}
}

func TestRender(t *testing.T) {
	got := Render(State{{Key: "a", Value: "1"}, {Key: "network.wlan0.psk", Value: "set"}})
	if want := "a = 1\nnetwork.wlan0.psk = set\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
