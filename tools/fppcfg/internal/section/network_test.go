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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/fake"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// networkTestAPI is the fake API, except that applying some interfaces hangs
// until the call's context ends, as on 10.4 when the address it came in on
// goes away.
type networkTestAPI struct {
	*fake.API
	hang map[string]bool // interfaces whose apply hangs
}

func (a *networkTestAPI) Do(ctx context.Context, method, path string, body io.Reader, ct string) (*target.Response, error) {
	name := strings.TrimSuffix(strings.TrimPrefix(path, "/api/network/interface/"), "/apply")
	if a.hang[name] && path == "/api/network/interface/"+name+"/apply" {
		a.API.Do(ctx, method, path, body, ct) // record it
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return a.API.Do(ctx, method, path, body, ct)
}

// networkTestHTTP answers plain HTTP requests (the checks at addresses other
// than the management one) and records their URLs.
type networkTestHTTP struct {
	mu     sync.Mutex
	urls   []string
	status int // 0: transport error
}

func (h *networkTestHTTP) RoundTrip(r *http.Request) (*http.Response, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.urls = append(h.urls, r.URL.String())
	if h.status == 0 {
		return nil, errors.New("connection refused")
	}
	return &http.Response{StatusCode: h.status, Body: io.NopCloser(strings.NewReader("{}")), Request: r}, nil
}

func networkTestSection(cfg *config.Controller) *networkSection {
	s := NewNetwork(cfg).(*networkSection)
	s.applyTimeout, s.verifyTimeout, s.poll = 50*time.Millisecond, 200*time.Millisecond, 10*time.Millisecond
	return s
}

// networkTestWiFi is the 10.4 test board's config with both interfaces
// moved: eth0 (the one the tool talks through) to a new static address and
// wlan0 onto WiFi.
func networkTestWiFi() *config.Controller {
	return &config.Controller{Network: config.Network{
		Interfaces: []config.Interface{
			{Name: "eth0", Proto: "static", Address: "192.168.1.50", Netmask: "255.255.255.0"},
			{Name: "wlan0", Proto: "static", Address: "10.0.0.5", Netmask: "255.255.255.0", SSID: "KIR Lights", PSK: "hunter22"},
		},
		Gateway:       "192.168.1.1",
		ManageGateway: true,
		DNS:           []string{"8.8.8.8", "8.8.4.4"},
		ManageDNS:     true,
	}}
}

func TestNetworkWant(t *testing.T) {
	got := keysTestRender(NewNetwork(networkTestWiFi()).Want())
	want := `network.eth0.proto = static
network.eth0.address = 192.168.1.50
network.eth0.netmask = 255.255.255.0
network.eth0.live = in effect
network.wlan0.proto = static
network.wlan0.address = 10.0.0.5
network.wlan0.netmask = 255.255.255.0
network.wlan0.ssid = KIR Lights
network.wlan0.psk = set
network.wlan0.hidden = false
network.wlan0.wpa3 = false
network.wlan0.live = in effect
network.gateway = 192.168.1.1
network.dns.count = 2
network.dns[0] = 8.8.8.8
network.dns[1] = 8.8.4.4
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if st := NewNetwork(&config.Controller{}).Want(); st != nil {
		t.Errorf("unmanaged: %v", st)
	}
}

func TestNetworkObserve104(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4", "/api/network/interface", "/api/network/gateway", "/api/network/dns")
	tg := &target.Target{API: api, Address: "192.168.1.20"}
	s := networkTestSection(networkTestWiFi())
	got, err := s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	want := `network.eth0.proto = dhcp
network.eth0.live = in effect
network.wlan0.proto = dhcp
network.wlan0.ssid =
network.wlan0.psk = not set
network.wlan0.hidden = false
network.wlan0.wpa3 = false
network.wlan0.live = not in effect
network.gateway =
network.dns.count = 0
`
	if r := strings.ReplaceAll(state.Render(got), " \n", "\n"); r != want {
		t.Errorf("got\n%s\nwant\n%s", r, want)
	}
	if s.mgmt != "eth0" || !s.globalGateway {
		t.Errorf("mgmt %q, global gateway %v", s.mgmt, s.globalGateway)
	}
}

func TestNetworkObserveMissingInterface(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4", "/api/network/interface")
	tg := &target.Target{API: api, Address: "192.168.1.20"}
	cfg := &config.Controller{Network: config.Network{Interfaces: []config.Interface{{Name: "eth1", Proto: "dhcp"}}}}
	_, err := networkTestSection(cfg).Observe(context.Background(), tg)
	if err == nil || !strings.Contains(err.Error(), "eth1") {
		t.Errorf("error %v", err)
	}
}

// networkTest8x is an FPP 8.x interface list: the gateway stored with wlan0,
// and no PSK in the list (the interface's own GET has it).
const networkTest8x = `[
 {"ifname":"eth0","addr_info":[{"family":"inet","local":"10.1.0.5","prefixlen":24},{"family":"inet6","local":"fe80::1","prefixlen":64}],
  "config":{"INTERFACE":"eth0","PROTO":"static","ADDRESS":"10.1.0.5","NETMASK":"255.255.255.0","GATEWAY":"","DHCPSERVER":"0","DHCPOFFSET":"100","DHCPPOOLSIZE":"50","ROUTEMETRIC":"0","IPFORWARDING":"0"}},
 {"ifname":"wlan0","addr_info":[{"family":"inet","local":"192.168.1.30","prefixlen":24}],
  "config":{"INTERFACE":"wlan0","PROTO":"static","ADDRESS":"192.168.1.30","NETMASK":"255.255.255.0","GATEWAY":"192.168.1.1","SSID":"KIR Lights","HIDDEN":"","WPA3":"1","BACKUPSSID":"","BACKUPHIDDEN":"","BACKUPWPA3":"","DHCPSERVER":"0","DHCPOFFSET":"100","DHCPPOOLSIZE":"50","ROUTEMETRIC":"0","IPFORWARDING":"0"}}
]`

const networkTest8xWlan0 = `{"INTERFACE":"wlan0","PROTO":"static","ADDRESS":"192.168.1.30","NETMASK":"255.255.255.0","GATEWAY":"192.168.1.1","SSID":"KIR Lights","PSK":"hunter22","HIDDEN":"","WPA3":"1","BACKUPSSID":"","BACKUPPSK":"spare","BACKUPHIDDEN":"","BACKUPWPA3":"","status":"OK"}`

func networkTest8xCfg(gateway string) *config.Controller {
	return &config.Controller{Network: config.Network{
		Interfaces: []config.Interface{
			{Name: "eth0", Proto: "static", Address: "10.1.0.5", Netmask: "255.255.255.0"},
			{Name: "wlan0", Proto: "static", Address: "192.168.1.30", Netmask: "255.255.255.0", SSID: "KIR Lights", PSK: "hunter22", WPA3: true},
		},
		Gateway:       gateway,
		ManageGateway: true,
	}}
}

func TestNetwork8xGateway(t *testing.T) {
	api := fake.NewAPI(t).
		Reply("GET", "/api/network/interface", 200, networkTest8x).
		Reply("GET", "/api/network/interface", 200, strings.Replace(networkTest8x, `"GATEWAY":"192.168.1.1"`, `"GATEWAY":"192.168.1.254"`, 1)).
		Reply("GET", "/api/network/interface/wlan0", 200, networkTest8xWlan0).
		Reply("GET", "/api/network/gateway", 404, "<html>not found</html>").
		Reply("POST", "/api/network/interface/wlan0", 200, `{"status":"OK"}`).
		Reply("POST", "/api/network/interface/wlan0/apply", 200, `{"status":"OK","output":[]}`).
		Reply("GET", "/api/fppd/version", 200, `{"version":"8.4.1"}`)
	h := &networkTestHTTP{status: 200}
	// The zone is the operator's interface; eth0 is found by the address.
	tg := &target.Target{API: api, Address: "fe80::1%enp3s0", HTTP: &http.Client{Transport: h}}
	s := networkTestSection(networkTest8xCfg("192.168.1.254"))
	have, err := s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	changes := state.Compare(s.Want(), have)
	if len(changes) != 1 || changes[0].Key != "network.gateway" || *changes[0].Have != "192.168.1.1" {
		t.Fatalf("changes %+v", changes)
	}
	if s.globalGateway || s.gatewayIface != "wlan0" || s.mgmt != "eth0" {
		t.Errorf("global %v, gateway interface %q, mgmt %q", s.globalGateway, s.gatewayIface, s.mgmt)
	}
	if _, err := s.Apply(context.Background(), tg, changes); err != nil {
		t.Fatal(err)
	}
	w := api.Writes()
	if len(w) != 2 || w[0].Path != "/api/network/interface/wlan0" || w[1].Path != "/api/network/interface/wlan0/apply" {
		t.Fatalf("writes %+v", w)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(w[0].Body), &body); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]any{
		"INTERFACE": "wlan0", "PROTO": "static", "ADDRESS": "192.168.1.30", "GATEWAY": "192.168.1.254",
		"PSK": "hunter22", "BACKUPPSK": "spare", "HIDDEN": false, "WPA3": true, "DHCPSERVER": false, "ROUTEMETRIC": "0",
	} {
		if body[k] != v {
			t.Errorf("%s = %#v, want %#v", k, body[k], v)
		}
	}
	if tg.Address != "fe80::1%enp3s0" || api.Address() != "" {
		t.Errorf("address moved to %q / %q", tg.Address, api.Address())
	}
	// Reached over link-local, so both static addresses are checked over
	// plain HTTP and the management address through the API.
	if !reflect.DeepEqual(h.urls, []string{"http://10.1.0.5/api/fppd/version", "http://192.168.1.30/api/fppd/version"}) {
		t.Errorf("checked %q", h.urls)
	}

	// No static interface holds the gateway.
	api = fake.NewAPI(t).
		Reply("GET", "/api/network/interface", 200, networkTest8x).
		Reply("GET", "/api/network/interface/wlan0", 200, networkTest8xWlan0).
		Reply("GET", "/api/network/gateway", 404, "")
	tg = &target.Target{API: api, Address: "10.1.0.5"}
	if _, err := networkTestSection(networkTest8xCfg("172.16.0.1")).Observe(context.Background(), tg); err == nil || !strings.Contains(err.Error(), "172.16.0.1") {
		t.Errorf("gateway outside every subnet: %v", err)
	}
}

func TestNetworkPSK(t *testing.T) {
	list104 := func(pskSet bool) string {
		set := "false"
		if pskSet {
			set = "true"
		}
		return `[{"ifname":"wlan0","addr_info":[],"config":{"INTERFACE":"wlan0","PROTO":"dhcp","SSID":"KIR Lights","HIDDEN":"","WPA3":"","PSKSet":` + set + `}}]`
	}
	tests := []struct {
		name   string
		list   string
		single string
		want   string
	}{
		{name: "8.x API gives the PSK", list: networkTest8x, single: networkTest8xWlan0, want: "set"},
		{name: "8.x API, empty PSK", list: networkTest8x, single: strings.Replace(networkTest8xWlan0, `"PSK":"hunter22"`, `"PSK":""`, 1), want: "not set"},
		{name: "10.4 set", list: list104(true), want: "set"},
		{name: "10.4 none set", list: list104(false), want: "not set"},
		{name: "nothing saved", list: `[{"ifname":"wlan0","addr_info":[],"config":null}]`, want: "not set"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := fake.NewAPI(t).Reply("GET", "/api/network/interface", 200, tc.list)
			if tc.single != "" {
				api.Reply("GET", "/api/network/interface/wlan0", 200, tc.single)
			}
			cfg := &config.Controller{Network: config.Network{Interfaces: []config.Interface{
				{Name: "wlan0", Proto: "dhcp", SSID: "KIR Lights", PSK: "hunter22"},
			}}}
			s := networkTestSection(cfg)
			have, err := s.Observe(context.Background(), &target.Target{API: api})
			if err != nil {
				t.Fatal(err)
			}
			if v, _ := have.Get("network.wlan0.psk"); v != tc.want {
				t.Errorf("got %q, want %q", v, tc.want)
			}
			if r := state.Render(have); strings.Contains(r, "hunter22") {
				t.Errorf("rendered the PSK:\n%s", r)
			}
		})
	}
}

// A PSK the config does not give is not managed: no entry, and a save
// leaves PSK out so FPP keeps the saved one.
func TestNetworkNoPSK(t *testing.T) {
	const list = `[{"ifname":"wlan0","addr_info":[],"config":{"INTERFACE":"wlan0","PROTO":"dhcp","SSID":"Old","HIDDEN":"","WPA3":"","PSKSet":true}}]`
	api := fake.NewAPI(t).Reply("GET", "/api/network/interface", 200, list)
	cfg := &config.Controller{Network: config.Network{Interfaces: []config.Interface{
		{Name: "wlan0", Proto: "dhcp", SSID: "KIR Lights"},
	}}}
	s := networkTestSection(cfg)
	have, err := s.Observe(context.Background(), &target.Target{API: api})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := have.Get("network.wlan0.psk"); ok {
		t.Errorf("observed a PSK entry: %v", have)
	}
	if _, ok := s.Want().Get("network.wlan0.psk"); ok {
		t.Errorf("wants a PSK entry: %v", s.Want())
	}
	body := s.saveBody(cfg.Network.Interfaces[0], s.ifaces["wlan0"])
	if _, ok := body["PSK"]; ok {
		t.Errorf("body has PSK: %v", body)
	}
	if body["SSID"] != "KIR Lights" {
		t.Errorf("body %v", body)
	}
}

// networkTest104After is the 10.4 board's interfaces once networkTestWiFi is
// in effect. The PSK reads only as set.
const networkTest104After = `[
 {"ifname":"eth0","addr_info":[{"family":"inet","local":"192.168.1.50","prefixlen":24}],
  "config":{"INTERFACE":"eth0","PROTO":"static","ADDRESS":"192.168.1.50","NETMASK":"255.255.255.0"}},
 {"ifname":"wlan0","addr_info":[{"family":"inet","local":"10.0.0.5","prefixlen":24}],
  "config":{"INTERFACE":"wlan0","PROTO":"static","ADDRESS":"10.0.0.5","NETMASK":"255.255.255.0","SSID":"KIR Lights","HIDDEN":"","WPA3":"","PSKSet":true}}
]`

// networkTestOpts varies networkTestApply's controller.
type networkTestOpts struct {
	wlanApplyStatus int    // wlan0's apply reply; 0 for 200 OK
	wlanApplyBody   string // with wlanApplyStatus
	hangWlan        bool   // wlan0's apply hangs too
	after           string // the interface list after the applies; "" for networkTest104After
	afterGateway    string // the gateway read after; "" for the config's
}

// networkTestApply observes the 10.4 board, which the tool reaches over eth0
// at 192.168.1.20, and applies networkTestWiFi to it.
func networkTestApply(t *testing.T, h *networkTestHTTP, o networkTestOpts) (*networkSection, *target.Target, *networkTestAPI, error) {
	t.Helper()
	if o.wlanApplyStatus == 0 {
		o.wlanApplyStatus, o.wlanApplyBody = 200, `{"status":"OK","output":[]}`
	}
	if o.after == "" {
		o.after = networkTest104After
	}
	if o.afterGateway == "" {
		o.afterGateway = "192.168.1.1"
	}
	f := fake.NewAPI(t).Recorded("fpp10.4", "/api/network/interface", "/api/network/gateway", "/api/network/dns").
		Reply("GET", "/api/network/interface", 200, o.after).
		Reply("GET", "/api/network/gateway", 200, `{"GATEWAY":"`+o.afterGateway+`"}`).
		Reply("GET", "/api/network/dns", 200, `{"DNS1":"8.8.8.8","DNS2":"8.8.4.4","status":"OK"}`).
		Reply("POST", "/api/network/interface/eth0", 200, `{"status":"OK"}`).
		Reply("POST", "/api/network/interface/wlan0", 200, `{"status":"OK"}`).
		Reply("POST", "/api/network/dns", 200, `{"status":"OK","DNS":{}}`).
		Reply("POST", "/api/network/gateway", 200, `{"status":"OK","GATEWAY":"192.168.1.1"}`).
		Reply("POST", "/api/network/interface/wlan0/apply", o.wlanApplyStatus, o.wlanApplyBody).
		Reply("POST", "/api/network/interface/eth0/apply", 200, ""). // hangs instead
		Reply("GET", "/api/fppd/version", 200, `{"version":"10.4"}`)
	api := &networkTestAPI{API: f, hang: map[string]bool{"eth0": true, "wlan0": o.hangWlan}}
	tg := &target.Target{API: api, Address: "192.168.1.20", HTTP: &http.Client{Transport: h}}
	s := networkTestSection(networkTestWiFi())
	have, err := s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(context.Background(), tg, state.Compare(s.Want(), have))
	return s, tg, api, err
}

func TestNetworkApply(t *testing.T) {
	h := &networkTestHTTP{status: 200}
	_, tg, api, err := networkTestApply(t, h, networkTestOpts{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, w := range api.Writes() {
		paths = append(paths, w.Path)
	}
	want := []string{
		"/api/network/interface/eth0",
		"/api/network/interface/wlan0",
		"/api/network/dns",
		"/api/network/gateway",
		"/api/network/interface/wlan0/apply",
		"/api/network/interface/eth0/apply", // management last; timed out
	}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("writes\n%q\nwant\n%q", paths, want)
	}
	w := api.Writes()
	var eth0, wlan0 map[string]any
	if err := json.Unmarshal([]byte(w[0].Body), &eth0); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(w[1].Body), &wlan0); err != nil {
		t.Fatal(err)
	}
	if _, ok := eth0["GATEWAY"]; ok {
		t.Error("10.4: GATEWAY sent with eth0")
	}
	for k, v := range map[string]any{"INTERFACE": "eth0", "PROTO": "static", "ADDRESS": "192.168.1.50", "NETMASK": "255.255.255.0", "DHCPSERVER": false, "DHCPOFFSET": "100"} {
		if eth0[k] != v {
			t.Errorf("eth0 %s = %#v, want %#v", k, eth0[k], v)
		}
	}
	for k, v := range map[string]any{"SSID": "KIR Lights", "PSK": "hunter22", "HIDDEN": false, "WPA3": false, "BACKUPSSID": "", "BACKUPHIDDEN": false, "ADDRESS": "10.0.0.5"} {
		if wlan0[k] != v {
			t.Errorf("wlan0 %s = %#v, want %#v", k, wlan0[k], v)
		}
	}
	for _, k := range []string{"PSKSet", "BACKUPPSKSet", "BACKUPPSK"} {
		if _, ok := wlan0[k]; ok {
			t.Errorf("wlan0 sent %s", k)
		}
	}
	if w[2].Body != `{"DNS1":"8.8.8.8","DNS2":"8.8.4.4"}` || w[3].Body != `{"GATEWAY":"192.168.1.1"}` {
		t.Errorf("dns %s, gateway %s", w[2].Body, w[3].Body)
	}
	if tg.Address != "192.168.1.50" || api.Address() != "192.168.1.50" {
		t.Errorf("address %q, API %q", tg.Address, api.Address())
	}
	if len(h.urls) == 0 || h.urls[0] != "http://10.0.0.5/api/fppd/version" {
		t.Errorf("checked %q", h.urls)
	}
}

func TestNetworkApplyVerifyFails(t *testing.T) {
	_, _, _, err := networkTestApply(t, &networkTestHTTP{}, networkTestOpts{})
	if err == nil || !strings.Contains(err.Error(), "wlan0") || !strings.Contains(err.Error(), "10.0.0.5") {
		t.Fatalf("error %v", err)
	}
	if strings.Contains(err.Error(), "hunter22") || strings.Contains(err.Error(), "eth0") {
		t.Errorf("error %v", err)
	}
}

func TestNetworkApplyDHCP(t *testing.T) {
	const before = `[{"ifname":"eth0","addr_info":[{"family":"inet","local":"10.1.0.5","prefixlen":24}],"config":{"INTERFACE":"eth0","PROTO":"static","ADDRESS":"10.1.0.5","NETMASK":"255.255.255.0"}},
{"ifname":"wlan0","addr_info":[],"config":null}]`
	const after = `[{"ifname":"eth0","addr_info":[{"family":"inet","local":"10.1.0.5","prefixlen":24}],"config":{"INTERFACE":"eth0","PROTO":"static","ADDRESS":"10.1.0.5","NETMASK":"255.255.255.0"}},
{"ifname":"wlan0","addr_info":[{"family":"inet","local":"192.168.1.77","prefixlen":24,"dynamic":true}],"config":{"INTERFACE":"wlan0","PROTO":"dhcp","SSID":"KIR Lights","HIDDEN":"","WPA3":"","PSKSet":true}}]`
	// The apply has answered, but wlan0 still holds an address it was not
	// leased.
	const unleased = `[{"ifname":"eth0","addr_info":[{"family":"inet","local":"10.1.0.5","prefixlen":24}],"config":{"INTERFACE":"eth0","PROTO":"static","ADDRESS":"10.1.0.5","NETMASK":"255.255.255.0"}},
{"ifname":"wlan0","addr_info":[{"family":"inet","local":"192.168.1.30","prefixlen":24}],"config":{"INTERFACE":"wlan0","PROTO":"dhcp","SSID":"KIR Lights","HIDDEN":"","WPA3":"","PSKSet":true}}]`
	cfg := &config.Controller{Network: config.Network{Interfaces: []config.Interface{
		{Name: "eth0", Proto: "static", Address: "10.1.0.5", Netmask: "255.255.255.0"},
		{Name: "wlan0", Proto: "dhcp", SSID: "KIR Lights", PSK: "hunter22"},
	}}}
	for _, tc := range []struct {
		name, after, wantErr string
	}{
		{name: "leased", after: after},
		{name: "no address", after: before, wantErr: "wlan0 has no DHCP lease"},
		{name: "address not leased", after: unleased, wantErr: "wlan0 has no DHCP lease"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := fake.NewAPI(t).
				Reply("GET", "/api/network/interface", 200, before).
				Reply("GET", "/api/network/interface", 200, tc.after).
				Reply("POST", "/api/network/interface/wlan0", 200, `{"status":"OK"}`).
				Reply("POST", "/api/network/interface/wlan0/apply", 200, `{"status":"OK"}`).
				Reply("GET", "/api/fppd/version", 200, `{}`)
			tg := &target.Target{API: api, Address: "10.1.0.5"}
			s := networkTestSection(cfg)
			have, err := s.Observe(context.Background(), tg)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Apply(context.Background(), tg, state.Compare(s.Want(), have))
			if tc.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error %v, want %q", err, tc.wantErr)
			}
			w := api.Writes()
			var body map[string]any
			json.Unmarshal([]byte(w[0].Body), &body)
			// Nothing was saved, so there is no key to keep: send it, and
			// the empty backup network.
			if body["PSK"] != "hunter22" || body["BACKUPPSK"] != "" || body["SSID"] != "KIR Lights" || body["PROTO"] != "dhcp" {
				t.Errorf("body %v", body)
			}
			if _, ok := body["ADDRESS"]; ok {
				t.Errorf("dhcp body has an address: %v", body)
			}
			if len(w) != 2 || w[1].Path != "/api/network/interface/wlan0/apply" {
				t.Errorf("writes %+v", w)
			}
		})
	}
}

func TestNetworkSaveRefused(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4", "/api/network/interface").
		Reply("POST", "/api/network/interface/wlan0", 200, `{"status":"ERROR: invalid characters in PSK hunter\"22"}`)
	tg := &target.Target{API: api, Address: "192.168.1.20"}
	cfg := &config.Controller{Network: config.Network{Interfaces: []config.Interface{
		{Name: "wlan0", Proto: "dhcp", SSID: "KIR Lights", PSK: `hunter"22`},
	}}}
	s := networkTestSection(cfg)
	have, err := s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(context.Background(), tg, state.Compare(s.Want(), have))
	if err == nil || !strings.Contains(err.Error(), "invalid characters") || strings.Contains(err.Error(), "hunter") {
		t.Fatalf("error %v", err)
	}
	if len(api.Writes()) != 1 {
		t.Errorf("went on after a refused save: %+v", api.Writes())
	}
}

func TestNetworkManagement(t *testing.T) {
	var items []networkListItem
	if err := json.Unmarshal([]byte(networkTest8x), &items); err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]string{
		"10.1.0.5": "eth0",
		// The same address, IPv4-mapped.
		"::ffff:10.1.0.5": "eth0",
		"192.168.1.30":    "wlan0",
		// The zone is the operator's interface: only the address counts.
		"fe80::1%enp3s0": "eth0",
		"fe80::1%wlan0":  "eth0",
		"fe80::9%wlan0":  "",
		"192.168.1.99":   "",
	} {
		if got := networkManagement(context.Background(), items, addr); got != want {
			t.Errorf("%s: %q, want %q", addr, got, want)
		}
	}
	// An address on two interfaces does not say which one the tool reaches.
	var both []networkListItem
	if err := json.Unmarshal([]byte(`[{"ifname":"eth0","addr_info":[{"family":"inet","local":"10.1.0.5","prefixlen":24}]},
{"ifname":"wlan0","addr_info":[{"family":"inet","local":"10.1.0.5","prefixlen":24}]}]`), &both); err != nil {
		t.Fatal(err)
	}
	if got := networkManagement(context.Background(), both, "10.1.0.5"); got != "" {
		t.Errorf("on two interfaces: %q", got)
	}
	// A hostname is resolved; localhost comes from /etc/hosts, offline.
	var lo []networkListItem
	if err := json.Unmarshal([]byte(`[{"ifname":"lo","addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8}]}]`), &lo); err != nil {
		t.Fatal(err)
	}
	if got := networkManagement(context.Background(), lo, "localhost"); got != "lo" {
		t.Errorf("localhost: %q, want lo", got)
	}
	if !networkInSubnet("192.168.1.1", "192.168.1.30", "255.255.255.0") || networkInSubnet("192.168.2.1", "192.168.1.30", "255.255.255.0") {
		t.Error("networkInSubnet")
	}
}

// networkTestStatic is a board whose eth0 is static at 10.1.0.5, with a
// link-local address beside it, and whose wlan0 is on 192.168.1.30.
const networkTestStatic = `[
 {"ifname":"eth0","addr_info":[{"family":"inet","local":"10.1.0.5","prefixlen":24},{"family":"inet6","local":"fe80::1","prefixlen":64}],
  "config":{"INTERFACE":"eth0","PROTO":"static","ADDRESS":"10.1.0.5","NETMASK":"255.255.255.0"}},
 {"ifname":"wlan0","addr_info":[{"family":"inet","local":"192.168.1.30","prefixlen":24}],
  "config":{"INTERFACE":"wlan0","PROTO":"dhcp","SSID":"KIR Lights","HIDDEN":"","WPA3":"","PSKSet":true}}
]`

func TestNetworkManagementToDHCP(t *testing.T) {
	cfg := &config.Controller{Network: config.Network{Interfaces: []config.Interface{
		{Name: "eth0", Proto: "dhcp"},
	}}}
	const after = `[{"ifname":"eth0","addr_info":[{"family":"inet","local":"10.1.0.77","prefixlen":24,"dynamic":true},{"family":"inet6","local":"fe80::1","prefixlen":64}],
  "config":{"INTERFACE":"eth0","PROTO":"dhcp"}}]`

	t.Run("over IPv4", func(t *testing.T) {
		api := fake.NewAPI(t).Reply("GET", "/api/network/interface", 200, networkTestStatic)
		tg := &target.Target{API: api, Address: "10.1.0.5"}
		s := networkTestSection(cfg)
		if _, err := s.Observe(context.Background(), tg); err != nil {
			t.Fatal(err)
		}
		if n := s.Notes(); len(n) != 1 || !strings.Contains(n[0], "eth0") || !strings.Contains(n[0], "--at") {
			t.Errorf("notes %q", n)
		}
		if len(api.Writes()) != 0 {
			t.Errorf("writes %+v", api.Writes())
		}
	})
	t.Run("over another interface", func(t *testing.T) {
		api := fake.NewAPI(t).Reply("GET", "/api/network/interface", 200, networkTestStatic)
		tg := &target.Target{API: api, Address: "192.168.1.30"}
		s := networkTestSection(cfg)
		if _, err := s.Observe(context.Background(), tg); err != nil {
			t.Fatal(err)
		}
		if s.mgmt != "wlan0" {
			t.Errorf("mgmt %q", s.mgmt)
		}
		if n := s.Notes(); len(n) != 0 {
			t.Errorf("notes %q", n)
		}
	})
	t.Run("over link-local", func(t *testing.T) {
		api := fake.NewAPI(t).
			Reply("GET", "/api/network/interface", 200, networkTestStatic).
			Reply("GET", "/api/network/interface", 200, after).
			Reply("POST", "/api/network/interface/eth0", 200, `{"status":"OK"}`).
			Reply("POST", "/api/network/interface/eth0/apply", 200, `{"status":"OK","output":[]}`).
			Reply("GET", "/api/fppd/version", 200, `{}`)
		tg := &target.Target{API: api, Address: "fe80::1%enp3s0"}
		s := networkTestSection(cfg)
		have, err := s.Observe(context.Background(), tg)
		if err != nil {
			t.Fatal(err)
		}
		if s.mgmt != "eth0" {
			t.Errorf("mgmt %q", s.mgmt)
		}
		if _, err := s.Apply(context.Background(), tg, state.Compare(s.Want(), have)); err != nil {
			t.Fatal(err)
		}
		if tg.Address != "fe80::1%enp3s0" || api.Address() != "" {
			t.Errorf("address moved to %q / %q", tg.Address, api.Address())
		}
	})
}

func TestNetworkApplyRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"500", 500, ""},
		{"error in a 200", 200, `{"status":"ERROR: invalid interface name"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, api, err := networkTestApply(t, &networkTestHTTP{status: 200}, networkTestOpts{wlanApplyStatus: tc.status, wlanApplyBody: tc.body})
			if err == nil || !strings.Contains(err.Error(), "applying interface wlan0") {
				t.Fatalf("error %v", err)
			}
			for _, w := range api.Writes() {
				if w.Path == "/api/network/interface/eth0/apply" {
					t.Error("applied the management interface after a refused apply")
				}
			}
		})
	}
}

func TestNetworkApplyTimeoutTolerated(t *testing.T) {
	// Both applies time out (the management one does in TestNetworkApply
	// too); the controller then answers and reads back as wanted.
	if _, _, _, err := networkTestApply(t, &networkTestHTTP{status: 200}, networkTestOpts{hangWlan: true}); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkApplyNotConverged(t *testing.T) {
	// Reachable at every address, but the WiFi network and the gateway did
	// not take.
	after := strings.Replace(networkTest104After, `"SSID":"KIR Lights"`, `"SSID":"Neighbour"`, 1)
	_, _, _, err := networkTestApply(t, &networkTestHTTP{status: 200}, networkTestOpts{after: after, afterGateway: "192.168.1.254"})
	if err == nil || !strings.Contains(err.Error(), "network.wlan0.ssid") || !strings.Contains(err.Error(), "network.gateway") {
		t.Fatalf("error %v", err)
	}
	// Keys only, never values, and the PSK, set after the apply, is not among them.
	for _, s := range []string{"Neighbour", "192.168.1.254", "psk", "hunter22"} {
		if strings.Contains(err.Error(), s) {
			t.Errorf("error mentions %q: %v", s, err)
		}
	}
}

func TestNetworkDNSWithoutInterfaces(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4", "/api/network/dns")
	tg := &target.Target{API: api, Address: "192.168.1.20"}
	s := networkTestSection(&config.Controller{Network: config.Network{DNS: []string{"8.8.8.8"}, ManageDNS: true}})
	have, err := s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), tg, state.Compare(s.Want(), have)); err == nil || !strings.Contains(err.Error(), "never take effect") {
		t.Fatalf("error %v", err)
	}
	if len(api.Writes()) != 0 {
		t.Errorf("saved anyway: %+v", api.Writes())
	}
}

func TestNetworkLivePrefix(t *testing.T) {
	stored := map[string]json.RawMessage{
		"PROTO": json.RawMessage(`"static"`), "ADDRESS": json.RawMessage(`"10.1.0.5"`), "NETMASK": json.RawMessage(`"255.255.255.0"`),
	}
	for prefix, want := range map[int]string{24: "in effect", 16: "not in effect"} {
		i := &networkIface{stored: stored, inet: []string{"10.1.0.5"}, prefix: map[string]int{"10.1.0.5": prefix}}
		if got := i.live(); got != want {
			t.Errorf("/%d: %q, want %q", prefix, got, want)
		}
	}
}

// A refused apply does not repeat the PSK either.
func TestNetworkApplyRefusedHidesPSK(t *testing.T) {
	api := fake.NewAPI(t).Recorded("fpp10.4", "/api/network/interface").
		Reply("POST", "/api/network/interface/wlan0", 200, `{"status":"OK"}`).
		Reply("POST", "/api/network/interface/wlan0/apply", 200, `{"status":"ERROR: wpa_supplicant rejected hunter\"22"}`)
	tg := &target.Target{API: api, Address: "192.168.1.20"}
	cfg := &config.Controller{Network: config.Network{Interfaces: []config.Interface{
		{Name: "wlan0", Proto: "dhcp", SSID: "KIR Lights", PSK: `hunter"22`},
	}}}
	s := networkTestSection(cfg)
	have, err := s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(context.Background(), tg, state.Compare(s.Want(), have))
	if err == nil || !strings.Contains(err.Error(), "rejected") || strings.Contains(err.Error(), "hunter") {
		t.Fatalf("error %v", err)
	}
}

// A config whose gateway and DNS are "dhcp" takes the stored ones away. On
// 10.4 nothing static uses the gateway, so only the DNS change needs the
// interface applied.
func TestNetworkFromDHCP104(t *testing.T) {
	const list = `[{"ifname":"eth0","addr_info":[{"family":"inet","local":"192.168.1.77","prefixlen":24,"dynamic":true}],"config":{"INTERFACE":"eth0","PROTO":"dhcp"}}]`
	cfg := &config.Controller{Network: config.Network{
		Interfaces:    []config.Interface{{Name: "eth0", Proto: "dhcp"}},
		ManageGateway: true,
		DNS:           []string{},
		ManageDNS:     true,
	}}
	api := fake.NewAPI(t).
		Reply("GET", "/api/network/interface", 200, list).
		Reply("GET", "/api/network/gateway", 200, `{"GATEWAY":"192.168.1.1"}`).
		Reply("GET", "/api/network/gateway", 200, `{"GATEWAY":""}`).
		Reply("GET", "/api/network/dns", 200, `{"DNS1":"8.8.8.8","DNS2":"8.8.4.4","status":"OK"}`).
		Reply("GET", "/api/network/dns", 200, `{"DNS1":"","DNS2":"","status":"OK"}`).
		Reply("POST", "/api/network/dns", 200, `{"status":"OK","DNS":{}}`).
		Reply("POST", "/api/network/gateway", 200, `{"status":"OK","GATEWAY":""}`).
		Reply("POST", "/api/network/interface/eth0/apply", 200, `{"status":"OK","output":[]}`).
		Reply("GET", "/api/fppd/version", 200, `{"version":"10.4"}`)
	tg := &target.Target{API: api, Address: "192.168.1.77", HTTP: &http.Client{Transport: &networkTestHTTP{status: 200}}}
	s := networkTestSection(cfg)
	if w, _ := s.Want().Get("network.gateway"); w != "dhcp" {
		t.Errorf("wanted gateway %q", w)
	}
	have, err := s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	changes := state.Compare(s.Want(), have)
	var keys []string
	for _, c := range changes {
		keys = append(keys, c.Key)
	}
	if !slices.Contains(keys, "network.gateway") || !slices.Contains(keys, "network.dns.count") {
		t.Fatalf("changes %q", keys)
	}
	if _, err := s.Apply(context.Background(), tg, changes); err != nil {
		t.Fatal(err)
	}
	var writes []string
	for _, w := range api.Writes() {
		writes = append(writes, w.Path+" "+w.Body)
	}
	want := []string{
		`/api/network/dns {"DNS1":"","DNS2":""}`,
		`/api/network/gateway {"GATEWAY":""}`,
		`/api/network/interface/eth0/apply `,
	}
	if !reflect.DeepEqual(writes, want) {
		t.Errorf("writes\n%q\nwant\n%q", writes, want)
	}
	// Both gone: the same config now finds nothing to change.
	have, err = s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	if changes := state.Compare(s.Want(), have); len(changes) != 0 {
		t.Errorf("still differs: %+v", changes)
	}
}

// On 8.x the gateway is stored with an interface, so taking it away
// rewrites the interface that has one.
func TestNetworkFromDHCP8x(t *testing.T) {
	api := fake.NewAPI(t).
		Reply("GET", "/api/network/interface", 200, networkTest8x).
		Reply("GET", "/api/network/interface", 200, strings.Replace(networkTest8x, `"GATEWAY":"192.168.1.1"`, `"GATEWAY":""`, 1)).
		Reply("GET", "/api/network/interface/wlan0", 200, networkTest8xWlan0).
		Reply("GET", "/api/network/gateway", 404, "").
		Reply("POST", "/api/network/interface/wlan0", 200, `{"status":"OK"}`).
		Reply("POST", "/api/network/interface/wlan0/apply", 200, `{"status":"OK","output":[]}`).
		Reply("GET", "/api/fppd/version", 200, `{"version":"8.4.1"}`)
	tg := &target.Target{API: api, Address: "fe80::1%enp3s0", HTTP: &http.Client{Transport: &networkTestHTTP{status: 200}}}
	s := networkTestSection(networkTest8xCfg(""))
	have, err := s.Observe(context.Background(), tg)
	if err != nil {
		t.Fatal(err)
	}
	changes := state.Compare(s.Want(), have)
	if len(changes) != 1 || changes[0].Key != "network.gateway" || *changes[0].Have != "192.168.1.1" || *changes[0].Want != "dhcp" {
		t.Fatalf("changes %+v", changes)
	}
	if _, err := s.Apply(context.Background(), tg, changes); err != nil {
		t.Fatal(err)
	}
	w := api.Writes()
	if len(w) != 2 || w[0].Path != "/api/network/interface/wlan0" || w[1].Path != "/api/network/interface/wlan0/apply" {
		t.Fatalf("writes %+v", w)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(w[0].Body), &body); err != nil {
		t.Fatal(err)
	}
	if gw, ok := body["GATEWAY"]; !ok || gw != "" {
		t.Errorf("GATEWAY = %#v, want it sent empty", body["GATEWAY"])
	}
}

// Taking only the gateway away still applies the interface the config
// names, and clears a gateway left in that interface's own config, which
// 10.4 would otherwise make the global one again.
func TestNetworkFromDHCPGatewayOnly(t *testing.T) {
	list := func(gw string) string {
		return `[{"ifname":"eth0","addr_info":[{"family":"inet","local":"192.168.1.77","prefixlen":24,"dynamic":true}],"config":{"INTERFACE":"eth0","PROTO":"dhcp","GATEWAY":"` + gw + `"}}]`
	}
	cfg := &config.Controller{Network: config.Network{
		Interfaces:    []config.Interface{{Name: "eth0", Proto: "dhcp"}},
		ManageGateway: true,
	}}
	for _, global := range []string{"192.168.1.1", ""} {
		api := fake.NewAPI(t).
			Reply("GET", "/api/network/interface", 200, list("192.168.1.254")).
			Reply("GET", "/api/network/interface", 200, list("")).
			Reply("GET", "/api/network/gateway", 200, `{"GATEWAY":"`+global+`"}`).
			Reply("GET", "/api/network/gateway", 200, `{"GATEWAY":""}`).
			Reply("POST", "/api/network/interface/eth0", 200, `{"status":"OK"}`).
			Reply("POST", "/api/network/gateway", 200, `{"status":"OK","GATEWAY":""}`).
			Reply("POST", "/api/network/interface/eth0/apply", 200, `{"status":"OK","output":[]}`).
			Reply("GET", "/api/fppd/version", 200, `{"version":"10.4"}`)
		tg := &target.Target{API: api, Address: "192.168.1.77", HTTP: &http.Client{Transport: &networkTestHTTP{status: 200}}}
		s := networkTestSection(cfg)
		have, err := s.Observe(context.Background(), tg)
		if err != nil {
			t.Fatal(err)
		}
		changes := state.Compare(s.Want(), have)
		wantHave := global
		if wantHave == "" {
			wantHave = "192.168.1.254" // the one left with eth0
		}
		if len(changes) != 1 || changes[0].Key != "network.gateway" || *changes[0].Have != wantHave {
			t.Fatalf("global %q: changes %+v", global, changes)
		}
		if _, err := s.Apply(context.Background(), tg, changes); err != nil {
			t.Fatal(err)
		}
		w := api.Writes()
		if len(w) != 3 || w[0].Path != "/api/network/interface/eth0" || w[1].Path+" "+w[1].Body != `/api/network/gateway {"GATEWAY":""}` ||
			w[2].Path != "/api/network/interface/eth0/apply" {
			t.Fatalf("global %q: writes %+v", global, w)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(w[0].Body), &body); err != nil {
			t.Fatal(err)
		}
		if gw, ok := body["GATEWAY"]; !ok || gw != "" {
			t.Errorf("global %q: eth0 GATEWAY = %#v, want it sent empty", global, body["GATEWAY"])
		}
	}
}
