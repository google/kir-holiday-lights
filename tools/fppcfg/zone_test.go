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

package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func testZoner(names []string, answering ...string) *zoner {
	return &zoner{
		interfaces: func() ([]string, error) { return names, nil },
		answers: func(_ context.Context, addr string) bool {
			for _, a := range answering {
				if strings.HasSuffix(addr, "%"+a) {
					return true
				}
			}
			return false
		},
		timeout: time.Second,
	}
}

func TestWithZone(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		addr      string
		ifs       []string
		answering []string
		want      string
		err       string
	}{
		{name: "IPv4 untouched", addr: "192.168.1.20", want: "192.168.1.20"},
		{name: "global IPv6 untouched", addr: "2001:db8::1", want: "2001:db8::1"},
		{name: "zone kept", addr: "fe80::1%eth9", ifs: []string{"enp3s0"}, want: "fe80::1%eth9"},
		{name: "only interface", addr: "fe80::1", ifs: []string{"enp3s0"}, want: "fe80::1%enp3s0"},
		{name: "the one that answers", addr: "fe80::1", ifs: []string{"wlan0", "enp3s0"}, answering: []string{"enp3s0"}, want: "fe80::1%enp3s0"},
		{name: "none", addr: "fe80::1", err: "no interface"},
		{name: "none answer", addr: "fe80::1", ifs: []string{"wlan0", "enp3s0"}, err: "does not answer through any of wlan0, enp3s0"},
		{name: "both answer", addr: "fe80::1", ifs: []string{"wlan0", "enp3s0"}, answering: []string{"wlan0", "enp3s0"}, err: "answers through wlan0 and enp3s0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testZoner(tc.ifs, tc.answering...).withZone(ctx, tc.addr)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
