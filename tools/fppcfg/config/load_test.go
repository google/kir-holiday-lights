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

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// setup is the part every valid config needs.
const setup = `
setup: {
	LegalJurisdiction: "US"
	statsPublish:      "Disabled"
	ShareCrashData:    0
	FetchVendorLogos:  false
	SendVendorSerial:  false
}
`

// writeDir writes files into a new directory and returns the path of
// station.cfg in it.
func writeDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "station.cfg")
}

func TestLoadFull(t *testing.T) {
	path := writeDir(t, map[string]string{
		// The shared package lets station.cfg use secrets.cue's _wifi.
		"defaults.cue": "package show\n" + setup + `
settings: {
	fppMode:          "remote"
	MultiSyncEnabled: true
	uiLevel:          *1 | int
	Latitude:         47.6784
}
network: {
	gateway: "192.168.1.1"
	dns: ["8.8.8.8", "8.8.4.4"]
}
`,
		"secrets.cue": `package show
passwords: {ui: "uipw", os: "ospw"}
_wifi: "wifipsk"
`,
		"station.cfg": `package show
version: "10.4"
address: "fe80::1"
settings: {
	AudioOutput:     "hw:1"
	HostDescription: "K16 N Tunnel"
	HostName:        "k16-tunnel"
	Service_rsync:   false
	Longitude:       -122
}
volume: 70
sshKeys: ["ssh-ed25519 AAAA one", "ssh-ed25519 AAAA two"]
network: interfaces: {
	wlan0: {proto: "dhcp", ssid: "Show", psk: _wifi, hidden: true}
	eth0: {proto: "static", address: "192.168.1.32", netmask: "255.255.255.0"}
}
`,
	})
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	vol := 70
	want := &Controller{
		Version: "10.4",
		Address: "fe80::1",
		Settings: []Setting{
			{"HostDescription", "K16 N Tunnel"},
			{"HostName", "k16-tunnel"},
			{"fppMode", "remote"},
			{"MultiSyncEnabled", "1"},
			{"Service_rsync", "0"},
			{"uiLevel", "1"},
			{"Latitude", "47.6784"},
			{"Longitude", "-122"},
			{"AudioOutput", "hw:1"},
		},
		Setup: []Setting{
			{"LegalJurisdiction", "US"},
			{"statsPublish", "Disabled"},
			{"ShareCrashData", "0"},
			{"FetchVendorLogos", "0"},
			{"SendVendorSerial", "0"},
		},
		Volume:     &vol,
		UIPassword: "uipw",
		OSPassword: "ospw",
		SSHKeys:    []string{"ssh-ed25519 AAAA one", "ssh-ed25519 AAAA two"},
		ManageKeys: true,
		Network: Network{
			Interfaces: []Interface{
				{Name: "eth0", Proto: "static", Address: "192.168.1.32", Netmask: "255.255.255.0"},
				{Name: "wlan0", Proto: "dhcp", SSID: "Show", PSK: "wifipsk", Hidden: true},
			},
			Gateway:       "192.168.1.1",
			ManageGateway: true,
			DNS:           []string{"8.8.8.8", "8.8.4.4"},
			ManageDNS:     true,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load:\n got %+v\nwant %+v", got, want)
	}
}

// settingValue returns the named setting's value, or "" when unset.
func settingValue(c *Controller, name string) string {
	for _, s := range c.Settings {
		if s.Name == name {
			return s.Value
		}
	}
	return ""
}

func TestLoadSharedFile(t *testing.T) {
	tests := []struct {
		name    string
		station string
		want    string // uiLevel, or "" when an error is expected
		wantErr []string
	}{
		{name: "default kept", station: setup, want: "1"},
		{name: "default overridden", station: setup + "settings: uiLevel: 3", want: "3"},
		{name: "same value unifies", station: setup + `settings: Locale: "USA"`, want: "1"},
		{
			name:    "different value conflicts",
			station: setup + `settings: Locale: "Global"`,
			wantErr: []string{"settings.Locale", "conflicting values", "defaults.cue", "station.cfg"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeDir(t, map[string]string{
				"defaults.cue": `settings: {uiLevel: *1 | int, Locale: "USA"}`,
				"station.cfg":  tt.station,
			})
			c, err := Load(path)
			if tt.wantErr != nil {
				checkErr(t, err, tt.wantErr, nil)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := settingValue(c, "uiLevel"); got != tt.want {
				t.Errorf("uiLevel = %q, want %q", got, tt.want)
			}
		})
	}
}

func checkErr(t *testing.T, err error, want, notWant []string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Load succeeded, want an error containing %q", want)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error lacks %q:\n%v", w, err)
		}
	}
	for _, w := range notWant {
		if strings.Contains(err.Error(), w) {
			t.Errorf("error contains %q:\n%v", w, err)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		want    []string
		notWant []string
	}{
		{
			name:  "unknown setting",
			files: map[string]string{"station.cfg": setup + "settings: bogus: 1"},
			want:  []string{"settings.bogus", "field not allowed", "station.cfg:"},
		},
		{
			name:  "unknown top-level field",
			files: map[string]string{"station.cfg": setup + "colour: 1"},
			want:  []string{"colour", "field not allowed", "station.cfg:"},
		},
		{
			name:  "missing setup field",
			files: map[string]string{"station.cfg": `setup: {LegalJurisdiction: "US", statsPublish: "Disabled", ShareCrashData: 0, FetchVendorLogos: false}`},
			want:  []string{"setup.SendVendorSerial", "required", "station.cfg:"},
		},
		{
			name:  "missing setup",
			files: map[string]string{"station.cfg": `settings: fppMode: "remote"`},
			want:  []string{"setup", "required"},
		},
		{
			name:  "bad hostname",
			files: map[string]string{"station.cfg": setup + `settings: HostName: "-bad"`},
			want:  []string{"settings.HostName", "station.cfg:"},
		},
		{
			name:  "long hostname",
			files: map[string]string{"station.cfg": setup + `settings: HostName: "a234567890123456789012345678901"`},
			want:  []string{"settings.HostName", "station.cfg:"},
		},
		{
			name:  "static without address",
			files: map[string]string{"station.cfg": setup + `network: interfaces: eth0: {proto: "static", netmask: "255.255.255.0"}`},
			want:  []string{"network.interfaces.eth0.address", "required", "station.cfg:"},
		},
		{
			name:  "bad netmask",
			files: map[string]string{"station.cfg": setup + `network: interfaces: eth0: {proto: "static", address: "10.0.0.2", netmask: "255.255.255"}`},
			want:  []string{"network.interfaces.eth0.netmask", "station.cfg:"},
		},
		{
			name:    "psk without ssid",
			files:   map[string]string{"station.cfg": setup + `network: interfaces: wlan0: {proto: "dhcp", psk: "wifisecret"}`},
			want:    []string{"network.interfaces.wlan0.psk", "without ssid", "station.cfg:"},
			notWant: []string{"wifisecret"},
		},
		{
			name:  "three dns servers",
			files: map[string]string{"station.cfg": setup + `network: dns: ["1.1.1.1", "8.8.8.8", "8.8.4.4"]`},
			want:  []string{"network.dns", "station.cfg:"},
		},
		{
			name:  "volume out of range",
			files: map[string]string{"station.cfg": setup + `volume: 101`},
			want:  []string{"volume", "station.cfg:"},
		},
		{
			name:  "key with a newline",
			files: map[string]string{"station.cfg": setup + `sshKeys: ["ssh-ed25519 A\nssh-ed25519 B"]`},
			want:  []string{"sshKeys.0", "station.cfg:"},
		},
		{
			name:  "empty password",
			files: map[string]string{"station.cfg": setup + `passwords: ui: ""`},
			want:  []string{"passwords.ui", "station.cfg:"},
		},
		{
			name: "conflicting passwords",
			files: map[string]string{
				"secrets.cue": `passwords: ui: "hunter2"`,
				"station.cfg": setup + `passwords: ui: "swordfish"`,
			},
			want:    []string{"passwords.ui", "secrets.cue:", "station.cfg:"},
			notWant: []string{"hunter2", "swordfish"},
		},
		{
			name: "conflicting psk",
			files: map[string]string{
				"secrets.cue": `network: interfaces: wlan0: psk: "hunter2"`,
				"station.cfg": setup + `network: interfaces: wlan0: {proto: "dhcp", ssid: "S", psk: "swordfish"}`,
			},
			want:    []string{"network.interfaces.wlan0.psk", "secrets.cue:"},
			notWant: []string{"hunter2", "swordfish"},
		},
		{
			name:    "password of the wrong shape",
			files:   map[string]string{"station.cfg": setup + `passwords: "hunter2"`},
			want:    []string{"passwords", "station.cfg:"},
			notWant: []string{"hunter2"},
		},
		{
			name: "conflicting hidden field",
			files: map[string]string{
				"secrets.cue": "package show\n" + `_wifi: "hunter2"`,
				"wifi.cue":    "package show\n" + `_wifi: "swordfish"`,
				"station.cfg": "package show\n" + setup + `network: interfaces: wlan0: {proto: "dhcp", ssid: "S", psk: _wifi}`,
			},
			want:    []string{"_wifi", "conflicting values", "secrets.cue:", "wifi.cue:"},
			notWant: []string{"hunter2", "swordfish"},
		},
		{
			name: "conflicting definition",
			files: map[string]string{
				"secrets.cue": "package show\n" + `#Secret: "hunter2"`,
				"other.cue":   "package show\n" + `#Secret: "swordfish"`,
				"station.cfg": "package show\n" + setup + `passwords: ui: #Secret`,
			},
			want:    []string{"#Secret", "conflicting values", "secrets.cue:", "other.cue:"},
			notWant: []string{"hunter2", "swordfish"},
		},
		{
			name: "conflicting hostname",
			files: map[string]string{
				"defaults.cue": `settings: HostName: "alpha"`,
				"station.cfg":  setup + `settings: HostName: "bravo"`,
			},
			want:    []string{"settings.HostName", "conflicting values", `"(hidden)"`, "defaults.cue:", "station.cfg:"},
			notWant: []string{"alpha", "bravo"},
		},
		{
			name:    "unterminated string",
			files:   map[string]string{"station.cfg": setup + "passwords: ui: \"hunter2\n"},
			want:    []string{"station.cfg:"},
			notWant: []string{"hunter2"},
		},
		{
			name:  "dns without interfaces",
			files: map[string]string{"station.cfg": setup + `network: dns: ["8.8.8.8"]`},
			want:  []string{"network.dns", "without network.interfaces", "station.cfg:"},
		},
		{
			name: "gateway without interfaces",
			files: map[string]string{
				"defaults.cue": `network: gateway: "192.168.1.1"`,
				"station.cfg":  setup + `network: interfaces: {}`,
			},
			want: []string{"network.gateway", "without network.interfaces", "defaults.cue:"},
		},
		{
			name:  "not concrete",
			files: map[string]string{"station.cfg": setup + `settings: HostDescription: string`},
			want:  []string{"settings.HostDescription"},
		},
		{
			name:  "syntax error",
			files: map[string]string{"station.cfg": setup + `settings: {`},
			want:  []string{"station.cfg:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeDir(t, tt.files))
			checkErr(t, err, tt.want, tt.notWant)
		})
	}
}

func TestHideLiterals(t *testing.T) {
	tests := []struct{ in, want string }{
		{`conflicting values "a" and "b"`, `conflicting values "(hidden)" and "(hidden)"`},
		{`invalid value "" (out of bound !="")`, `invalid value "" (out of bound !="")`},
		{`escaped "a\"b" end`, `escaped "(hidden)" end`},
		{`raw #"a"b"# end`, `raw "(hidden)" end`},
		{"multi \"\"\"\n\tsecret\n\t\"\"\" end", `multi "(hidden)" end`},
		{`bytes 'secret' and {a:'x'}`, `bytes "(hidden)" and {a:"(hidden)"}`},
		{`expected ':', found 'STRING' "secret"`, `expected ':', found 'STRING' "(hidden)"`},
		{`can't be "x"`, `can't be "(hidden)"`},
		{`#Secret: invalid`, `#Secret: invalid`},
		{`unclosed "secret and more`, `unclosed "(hidden)"`},
	}
	for _, tt := range tests {
		if got := hideLiterals(tt.in); got != tt.want {
			t.Errorf("hideLiterals(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLoadManageFlags(t *testing.T) {
	tests := []struct {
		name                  string
		extra                 string
		keys, processors, dns bool
	}{
		{name: "absent", extra: ""},
		{
			name:  "empty",
			extra: "sshKeys: []\noutputProcessors: []\nnetwork: {dns: [], interfaces: eth0: proto: \"dhcp\"}",
			keys:  true, processors: true, dns: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Load(writeDir(t, map[string]string{"station.cfg": setup + tt.extra}))
			if err != nil {
				t.Fatal(err)
			}
			if c.ManageKeys != tt.keys || c.ManageProcessors != tt.processors || c.Network.ManageDNS != tt.dns {
				t.Errorf("ManageKeys, ManageProcessors, ManageDNS = %v, %v, %v; want %v, %v, %v",
					c.ManageKeys, c.ManageProcessors, c.Network.ManageDNS, tt.keys, tt.processors, tt.dns)
			}
			if tt.keys && (len(c.SSHKeys) != 0 || len(c.OutputProcessors) != 0 || len(c.Network.DNS) != 0) {
				t.Errorf("empty lists came back non-empty: %+v", c)
			}
		})
	}
}

func TestLoadOutputProcessors(t *testing.T) {
	c, err := Load(writeDir(t, map[string]string{"station.cfg": setup + `
outputProcessors: [
	{type: "Remap", source: 10, destination: 1, count: 3, loops: 1, reverse: 0, active: 1, description: "a <b> & c"},
	{type: "Brightness", start: 1, count: 300, brightness: 50.5, gamma: 2.2, active: 1, nested: {z: 1, a: [2, 1]}},
]`}))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"active":1,"count":3,"description":"a <b> & c","destination":1,"loops":1,"reverse":0,"source":10,"type":"Remap"}`,
		`{"active":1,"brightness":50.5,"count":300,"gamma":2.2,"nested":{"a":[2,1],"z":1},"start":1,"type":"Brightness"}`,
	}
	if !reflect.DeepEqual(c.OutputProcessors, want) {
		t.Errorf("OutputProcessors:\n got %q\nwant %q", c.OutputProcessors, want)
	}
	if !c.ManageProcessors {
		t.Error("ManageProcessors is false")
	}
}

func TestLoadProxies(t *testing.T) {
	tests := []struct {
		name   string
		extra  string
		want   []Proxy
		manage bool
	}{
		{name: "absent"},
		{name: "empty", extra: "proxies: []", want: []Proxy{}, manage: true},
		{
			name:   "two",
			extra:  `proxies: [{host: "192.168.1.155", description: "Giga \"Tree\" > 1"}, {host: "arch.local"}]`,
			want:   []Proxy{{"192.168.1.155", `Giga "Tree" > 1`}, {"arch.local", ""}},
			manage: true,
		},
		{
			name:   "longest description",
			extra:  `proxies: [{host: "a", description: "` + strings.Repeat("x", 128) + `"}]`,
			want:   []Proxy{{"a", strings.Repeat("x", 128)}},
			manage: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Load(writeDir(t, map[string]string{"station.cfg": setup + tt.extra}))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c.Proxies, tt.want) || c.ManageProxies != tt.manage {
				t.Errorf("Proxies, ManageProxies = %#v, %v; want %#v, %v", c.Proxies, c.ManageProxies, tt.want, tt.manage)
			}
		})
	}
}

func TestLoadProxyErrors(t *testing.T) {
	tests := []struct {
		name, proxy string
	}{
		{"no host", `{description: "x"}`},
		{"host with port", `{host: "192.168.1.155:8080"}`},
		{"host with slash", `{host: "a/b"}`},
		{"tag", `{host: "a", description: "<b>"}`},
		{"line break", `{host: "a", description: "a\nb"}`},
		{"url", `{host: "a", description: "see http://a"}`},
		{"long", `{host: "a", description: "` + strings.Repeat("x", 129) + `"}`},
		{"unknown field", `{host: "a", port: 80}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeDir(t, map[string]string{"station.cfg": setup + "proxies: [" + tt.proxy + "]"}))
			// The real problem only, without what "| null" adds.
			checkErr(t, err, []string{"proxies.0", "station.cfg:"}, []string{"empty disjunction", "null"})
		})
	}
}

func TestLoadOtherCfgIgnored(t *testing.T) {
	path := writeDir(t, map[string]string{
		"station.cfg": setup,
		"other.cfg":   `settings: bogus: true`,
	})
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "other.cfg")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("other.cfg was loaded: %v", err)
	}
}

// A config named *.cue is the config itself, not loaded a second time.
func TestLoadCuePath(t *testing.T) {
	dir := filepath.Dir(writeDir(t, map[string]string{"station.cue": setup + "volume: 5"}))
	c, err := Load(filepath.Join(dir, "station.cue"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Volume == nil || *c.Volume != 5 {
		t.Errorf("Volume = %v, want 5", c.Volume)
	}
}

func TestLoadExample(t *testing.T) {
	dir := t.TempDir()
	copies := map[string]string{
		"defaults.cue":        "defaults.cue",
		"station.cfg":         "station.cfg",
		"standby.cfg":         "standby.cfg",
		"secrets.cue.example": "secrets.cue",
	}
	for from, to := range copies {
		b, err := os.ReadFile(filepath.Join("..", "docs", "example", from))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, to), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(filepath.Join(dir, "station.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{
		"HostDescription":    "K16 N Tunnel",
		"fppMode":            "remote",
		"bridgeDataPriority": "Prioritize Sequence",
		"uiLevel":            "1",
		"Latitude":           "47.6784",
		"Longitude":          "-122.1857",
	}
	for name, want := range checks {
		if got := settingValue(c, name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if len(c.Setup) != 5 || c.UIPassword == "" {
		t.Errorf("setup or passwords missing: %+v", c)
	}
	eth0 := Interface{Name: "eth0", Proto: "static", Address: "192.168.1.32", Netmask: "255.255.255.0"}
	if len(c.Network.Interfaces) != 1 || c.Network.Interfaces[0] != eth0 {
		t.Errorf("Interfaces = %+v, want [%+v]", c.Network.Interfaces, eth0)
	}
	if c.Network.Gateway != "192.168.1.1" || len(c.Network.DNS) != 2 {
		t.Errorf("station network = %+v", c.Network)
	}

	// The standby takes the gateway and DNS defaults back.
	c, err = Load(filepath.Join(dir, "standby.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	n := c.Network
	if !n.ManageGateway || n.Gateway != "" || !n.ManageDNS || len(n.DNS) != 0 || settingValue(c, "fppMode") != "remote" {
		t.Errorf("standby = %+v", c)
	}
}

func TestManagedSettings(t *testing.T) {
	got := ManagedSettings()
	if len(got) < 2 || got[0] != "HostDescription" || got[len(got)-1] != "AudioOutput" {
		t.Errorf("ManagedSettings = %q", got)
	}
	got[0] = "changed"
	if ManagedSettings()[0] != "HostDescription" {
		t.Error("ManagedSettings shares its slice with callers")
	}
}

// A null drops a value the shared file gives as a default, and reads as if
// the field had never been written.
func TestLoadNullUnsets(t *testing.T) {
	path := writeDir(t, map[string]string{
		"defaults.cue": setup + `
version: *"10.4" | _
address: *"fe80::1" | _
volume:  *70 | _
settings: {
	fppMode:     "remote"
	ntpServer:   *"time.google.com" | _
	uiLevel:     *1 | _
	AudioOutput: *"hw:1" | _
}
passwords: {ui: *"uipw" | _, os: *"ospw" | _}
sshKeys: *["ssh-ed25519 AAAA one"] | _
outputProcessors: *[{type: "Remap"}] | _
proxies: *[{host: "192.168.1.155"}] | _
network: {
	gateway: *"192.168.1.1" | _
	dns:     *["8.8.8.8", "8.8.4.4"] | _
	interfaces: wlan0: *{proto: "dhcp", ssid: "Show"} | _
}
`,
		"station.cfg": `
version: null
address: null
volume:  null
settings: {ntpServer: null, uiLevel: null, AudioOutput: null}
passwords: {ui: null, os: null}
sshKeys: null
outputProcessors: null
proxies: null
network: {
	gateway: null
	dns:     null
	interfaces: {wlan0: null, eth0: proto: "dhcp"}
}
`,
	})
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := &Controller{
		Settings: []Setting{{"fppMode", "remote"}},
		Setup:    got.Setup,
		Network:  Network{Interfaces: []Interface{{Name: "eth0", Proto: "dhcp"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// Without the null the same defaults apply.
func TestLoadNullDefaultKept(t *testing.T) {
	path := writeDir(t, map[string]string{
		"defaults.cue": setup + `
volume: *70 | _
network: {
	gateway: *"192.168.1.1" | _
	dns:     *["8.8.8.8"] | _
}
`,
		"station.cfg": `network: interfaces: eth0: {proto: "static", address: "192.168.1.32", netmask: "255.255.255.0"}`,
	})
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Volume == nil || *got.Volume != 70 || got.Network.Gateway != "192.168.1.1" ||
		!got.Network.ManageDNS || !reflect.DeepEqual(got.Network.DNS, []string{"8.8.8.8"}) {
		t.Errorf("defaults lost: %+v", got)
	}
}

func TestLoadNullErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "null against a value that is not a default",
			files: map[string]string{
				"defaults.cue": `network: gateway: "192.168.1.1"`,
				"station.cfg":  setup + `network: {gateway: null, interfaces: eth0: proto: "dhcp"}`,
			},
			want: []string{"network.gateway", "defaults.cue:", "station.cfg:"},
		},
		{
			name:  "null setup value",
			files: map[string]string{"station.cfg": strings.Replace(setup, `"US"`, "null", 1)},
			want:  []string{"setup.LegalJurisdiction", "station.cfg:"},
		},
		{
			// The other field's error must not swallow this one's.
			name:  "null setup value beside another error",
			files: map[string]string{"station.cfg": strings.Replace(setup, `"US"`, "null", 1) + "volume: 101"},
			want:  []string{"setup.LegalJurisdiction", "volume: invalid value"},
		},
		{
			name: "gateway with only a null interface",
			files: map[string]string{
				"station.cfg": setup + `network: {gateway: "192.168.1.1", interfaces: wlan0: null}`,
			},
			want: []string{"network.gateway", "without network.interfaces"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeDir(t, tt.files))
			checkErr(t, err, tt.want, nil)
			if err != nil {
				t.Log(err)
			}
		})
	}
}

// "dhcp" for the gateway or DNS is managed, with nothing of our own.
func TestLoadFromDHCP(t *testing.T) {
	path := writeDir(t, map[string]string{
		"defaults.cue": setup + `
network: {
	gateway: *"192.168.1.1" | _
	dns:     *["8.8.8.8", "8.8.4.4"] | _
}
`,
		"station.cfg": `network: {gateway: "dhcp", dns: "dhcp", interfaces: eth0: proto: "dhcp"}`,
	})
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Network{
		Interfaces:    []Interface{{Name: "eth0", Proto: "dhcp"}},
		ManageGateway: true,
		DNS:           []string{},
		ManageDNS:     true,
	}
	if !reflect.DeepEqual(got.Network, want) {
		t.Errorf("got  %+v\nwant %+v", got.Network, want)
	}

	_, err = Load(writeDir(t, map[string]string{
		"station.cfg": setup + `network: {gateway: "dhcpp", interfaces: eth0: proto: "dhcp"}`,
	}))
	checkErr(t, err, []string{"network.gateway", "station.cfg:"}, nil)
}
