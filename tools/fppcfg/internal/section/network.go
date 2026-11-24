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
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/kir-holiday-lights/tools/fppcfg/config"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/state"
	"github.com/google/kir-holiday-lights/tools/fppcfg/internal/target"
)

// networkSection manages the interfaces the config names, the gateway and
// DNS. It is applied last: after an fppd restart, before a reboot.
//
// Apply runs in three steps (docs/design.md): save everything, which changes
// nothing live; apply the interfaces that need it, the one the tool talks
// through last; then check that the controller answers where the config
// says it should. An apply's own reply proves nothing either way: it times
// out when the address it came in on goes away, and answers 200 on 8.x
// before the old address stops working.
type networkSection struct {
	cfg *config.Controller
	// applyTimeout bounds each apply call; verifyTimeout the checks after;
	// poll is how often the checks look.
	applyTimeout, verifyTimeout, poll time.Duration

	// Learned by Observe for Apply.
	ifaces map[string]*networkIface
	// globalGateway is true when the controller has /api/network/gateway
	// (10.4); otherwise the gateway is stored in gatewayIface's config (8.x).
	globalGateway bool
	gatewayIface  string
	// mgmt is the interface the tool talks through, or "" when unknown.
	mgmt string
	// notes are warnings from Observe for the engine to print.
	notes []string
}

// networkIface is one interface as the controller reports it.
type networkIface struct {
	inet []string // live IPv4 addresses
	// leased is true when one of them comes from DHCP (marked dynamic).
	leased bool
	// prefix is each live IPv4 address's prefix length.
	prefix map[string]int
	// stored is the saved config, or nil when there is none.
	stored map[string]json.RawMessage
	// leases are the static DHCP leases, kept when the interface serves
	// DHCP so a save does not drop them.
	leases json.RawMessage
}

func NewNetwork(cfg *config.Controller) Section {
	return &networkSection{
		cfg:           cfg,
		applyTimeout:  40 * time.Second,
		verifyTimeout: 60 * time.Second,
		poll:          2 * time.Second,
	}
}

func (s *networkSection) Name() string { return "network" }

func (s *networkSection) Notes() []string { return s.notes }

// networkBoolKeys are the stored fields FPP takes as JSON booleans.
var networkBoolKeys = map[string]bool{
	"DHCPSERVER": true, "HIDDEN": true, "WPA3": true, "BACKUPHIDDEN": true, "BACKUPWPA3": true,
}

func networkKey(iface, field string) string { return "network." + iface + "." + field }

func (s *networkSection) Want() state.State {
	n := s.cfg.Network
	var st state.State
	for _, i := range n.Interfaces {
		st = append(st, state.Entry{Key: networkKey(i.Name, "proto"), Value: i.Proto})
		if i.Proto == "static" {
			st = append(st,
				state.Entry{Key: networkKey(i.Name, "address"), Value: i.Address},
				state.Entry{Key: networkKey(i.Name, "netmask"), Value: i.Netmask})
		}
		if i.SSID != "" {
			st = append(st, state.Entry{Key: networkKey(i.Name, "ssid"), Value: i.SSID})
			if i.PSK != "" {
				st = append(st, state.Entry{Key: networkKey(i.Name, "psk"), Value: "set"})
			}
			st = append(st,
				state.Entry{Key: networkKey(i.Name, "hidden"), Value: strconv.FormatBool(i.Hidden)},
				state.Entry{Key: networkKey(i.Name, "wpa3"), Value: strconv.FormatBool(i.WPA3)})
		}
		st = append(st, state.Entry{Key: networkKey(i.Name, "live"), Value: "in effect"})
	}
	if n.ManageGateway {
		st = append(st, state.Entry{Key: "network.gateway", Value: networkGatewayValue(n.Gateway)})
	}
	if n.ManageDNS {
		st = append(st, state.List("network.dns", n.DNS)...)
	}
	return st
}

// networkGatewayValue is the gateway's entry for a config that wants none
// stored: no gateway reads as the config's word for it, "dhcp".
func networkGatewayValue(gw string) string {
	if gw == "" {
		return "dhcp"
	}
	return gw
}

// networkTruth normalises FPP's booleans, which read back as "" for false
// and "1" or "true" for true.
func networkTruth(v string) bool {
	return v == "1" || strings.EqualFold(v, "true")
}

func (i *networkIface) get(k string) string { return target.Scalar(i.stored[k]) }

// live reports whether the stored config is the one running: for a static
// interface, its address is up with the stored netmask's prefix length.
func (i *networkIface) live() string {
	ok := false
	switch i.get("PROTO") {
	case "static":
		a := i.get("ADDRESS")
		bits, valid := networkPrefixLen(i.get("NETMASK"))
		for _, l := range i.inet {
			ok = ok || (valid && l == a && i.prefix[l] == bits)
		}
	case "dhcp":
		ok = len(i.inet) > 0
	}
	if ok {
		return "in effect"
	}
	return "not in effect"
}

// networkPrefixLen turns a dotted IPv4 netmask into its prefix length.
func networkPrefixLen(mask string) (int, bool) {
	m := net.ParseIP(mask).To4()
	if m == nil {
		return 0, false
	}
	bits, size := net.IPMask(m).Size()
	return bits, size == 32
}

type networkListItem struct {
	Ifname   string `json:"ifname"`
	AddrInfo []struct {
		Family    string `json:"family"`
		Local     string `json:"local"`
		Prefixlen int    `json:"prefixlen"`
		Dynamic   bool   `json:"dynamic"`
	} `json:"addr_info"`
	Config map[string]json.RawMessage `json:"config"`
}

// networkList reads the controller's interfaces by name, and the raw items.
func networkList(ctx context.Context, api target.API) (map[string]*networkIface, []networkListItem, error) {
	var items []networkListItem
	if err := target.GetJSON(ctx, api, "/api/network/interface", &items); err != nil {
		return nil, nil, fmt.Errorf("reading the interfaces: %w", err)
	}
	out := make(map[string]*networkIface, len(items))
	for _, it := range items {
		ifc := &networkIface{stored: it.Config, prefix: map[string]int{}}
		for _, a := range it.AddrInfo {
			if a.Family == "inet" {
				ifc.inet = append(ifc.inet, a.Local)
				ifc.leased = ifc.leased || a.Dynamic
				ifc.prefix[a.Local] = a.Prefixlen
			}
		}
		out[it.Ifname] = ifc
	}
	return out, items, nil
}

// networkManagement finds the interface that holds addr, by matching the
// address against each interface's live ones. A hostname is resolved first;
// one whose addresses are on more than one interface gives "", as the tool
// cannot tell which it reaches. A zone ("fe80::1%enp3s0") is the operator's
// own interface, not the controller's, so it is ignored.
func networkManagement(ctx context.Context, items []networkListItem, addr string) string {
	var ips []netip.Addr
	if ip, ok := netAddr(addr); ok {
		ips = []netip.Addr{ip}
	} else if found, err := net.DefaultResolver.LookupNetIP(ctx, "ip", addr); err == nil {
		for _, ip := range found {
			ips = append(ips, ip.WithZone("").Unmap())
		}
	}
	found := ""
	for _, it := range items {
		for _, a := range it.AddrInfo {
			l, ok := netAddr(a.Local)
			if !ok || !slices.Contains(ips, l) {
				continue
			}
			if found != "" && found != it.Ifname {
				return ""
			}
			found = it.Ifname
		}
	}
	return found
}

func (s *networkSection) Observe(ctx context.Context, t *target.Target) (state.State, error) {
	return s.observe(ctx, t, false)
}

// observe reads the controller's network state into s. After an apply
// (after is true) it does not check the management interface.
func (s *networkSection) observe(ctx context.Context, t *target.Target, after bool) (state.State, error) {
	n := s.cfg.Network
	var st state.State
	s.ifaces = map[string]*networkIface{}
	s.mgmt = ""
	s.notes = nil

	if len(n.Interfaces) > 0 || n.ManageGateway {
		all, items, err := networkList(ctx, t.API)
		if err != nil {
			return nil, err
		}
		s.mgmt = networkManagement(ctx, items, t.Address)
		if s.mgmt == "" && len(n.Interfaces) > 0 && !after {
			s.notes = append(s.notes, fmt.Sprintf("warning: cannot tell which interface %s reaches, "+
				"so it is not applied last and a move of it to DHCP is not flagged", t.Address))
		}
		for _, c := range n.Interfaces {
			ifc, ok := all[c.Name]
			if !ok {
				return nil, fmt.Errorf("the config names interface %s, which the controller does not have", c.Name)
			}
			if !after {
				if note := networkCheckManagement(c, ifc, s.mgmt, t.Address); note != "" {
					s.notes = append(s.notes, note)
				}
			}
			s.ifaces[c.Name] = ifc
			entries, err := s.observeIface(ctx, t, c, ifc)
			if err != nil {
				return nil, err
			}
			st = append(st, entries...)
		}
	}

	if n.ManageGateway {
		have, err := s.observeGateway(ctx, t)
		if err != nil {
			return nil, err
		}
		if n.Gateway == "" {
			// Say "none stored" the way the config does.
			have = networkGatewayValue(have)
		}
		st = append(st, state.Entry{Key: "network.gateway", Value: have})
	}

	if n.ManageDNS {
		var dns map[string]json.RawMessage
		if err := target.GetJSON(ctx, t.API, "/api/network/dns", &dns); err != nil {
			return nil, fmt.Errorf("reading DNS: %w", err)
		}
		var have []string
		for _, k := range []string{"DNS1", "DNS2"} {
			if v := target.Scalar(dns[k]); v != "" {
				have = append(have, v)
			}
		}
		st = append(st, state.List("network.dns", have)...)
	}
	return st, nil
}

// networkCheckManagement warns when the config moves the interface the tool
// talks through, at an IPv4 address, onto DHCP: the tool cannot know where
// the controller will be, so the check after the apply will fail. An IPv6
// link-local address survives the move, and an address on another interface
// is not affected.
func networkCheckManagement(c config.Interface, ifc *networkIface, mgmt, addr string) string {
	if c.Name != mgmt || c.Proto != "dhcp" || ifc.get("PROTO") == "dhcp" {
		return ""
	}
	ip, ok := netAddr(addr)
	if !ok {
		return fmt.Sprintf("warning: the config moves interface %s to DHCP, and the tool reaches the controller "+
			"through it as %s; the check after the apply passes only if that name follows the new address", c.Name, addr)
	}
	if !ip.Is4() {
		return ""
	}
	host := ip.String()
	return fmt.Sprintf("warning: the config moves interface %s to DHCP, and the tool reaches the controller "+
		"through it at %s, so it cannot tell where the controller goes and the check after the apply will fail; "+
		"--at its IPv6 link-local address or an address on another interface avoids this", c.Name, host)
}

func (s *networkSection) observeIface(ctx context.Context, t *target.Target, c config.Interface, ifc *networkIface) (state.State, error) {
	var st state.State
	add := func(field, v string) {
		st = append(st, state.Entry{Key: networkKey(c.Name, field), Value: v})
	}
	proto := ifc.get("PROTO")
	add("proto", proto)
	if proto == "static" || ifc.get("ADDRESS") != "" {
		add("address", ifc.get("ADDRESS"))
		add("netmask", ifc.get("NETMASK"))
	}

	// The interface on its own carries what the list may not: the static
	// leases, and on 8.x the WiFi keys in plaintext. A save must send the
	// keys back, or 8.x would blank them.
	_, hasPSK := ifc.stored["PSK"]
	_, hasPSKSet := ifc.stored["PSKSet"]
	wifiKeys := ifc.stored != nil && strings.HasPrefix(c.Name, "wl") && !hasPSK && !hasPSKSet
	if networkTruth(ifc.get("DHCPSERVER")) || wifiKeys {
		var single map[string]json.RawMessage
		if err := target.GetJSON(ctx, t.API, "/api/network/interface/"+c.Name, &single); err != nil {
			return nil, fmt.Errorf("reading interface %s: %w", c.Name, err)
		}
		ifc.leases = single["StaticLeases"]
		if wifiKeys {
			for _, k := range []string{"PSK", "BACKUPPSK", "PSKSet", "BACKUPPSKSet"} {
				if v, ok := single[k]; ok {
					ifc.stored[k] = v
				}
			}
		}
	}

	if c.SSID != "" {
		add("ssid", ifc.get("SSID"))
		if c.PSK != "" {
			add("psk", ifc.pskSet())
		}
		add("hidden", strconv.FormatBool(networkTruth(ifc.get("HIDDEN"))))
		add("wpa3", strconv.FormatBool(networkTruth(ifc.get("WPA3"))))
	}
	add("live", ifc.live())
	return st, nil
}

// pskSet says whether a PSK is saved: "set" or "not set". FPP 10.4 reports
// only PSKSet; 8.x returns the PSK itself, which is looked at for being
// empty and never compared or kept in state.
func (i *networkIface) pskSet() string {
	if networkTruth(i.get("PSKSet")) || i.get("PSK") != "" {
		return "set"
	}
	return "not set"
}

// observeGateway probes for the global gateway call (10.4). Without it (8.x)
// the gateway is stored with the static interface whose subnet holds it.
//
// When the config wants none, a gateway stored with any interface the
// config names counts as one, on either kind: 8.x keeps it there, and 10.4
// takes an old one from there whenever its global gateway is empty.
func (s *networkSection) observeGateway(ctx context.Context, t *target.Target) (string, error) {
	const path = "/api/network/gateway"
	resp, err := t.API.Do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return "", fmt.Errorf("reading the gateway: %w", err)
	}
	if resp.Status == http.StatusNotFound {
		s.globalGateway = false
		s.gatewayIface = ""
		gw := s.cfg.Network.Gateway
		if gw == "" {
			return s.leftoverGateway(), nil
		}
		for _, c := range s.cfg.Network.Interfaces {
			if c.Proto == "static" && networkInSubnet(gw, c.Address, c.Netmask) {
				s.gatewayIface = c.Name
				return s.ifaces[c.Name].get("GATEWAY"), nil
			}
		}
		return "", fmt.Errorf("this FPP keeps the gateway with an interface, and no static interface in the config has %s in its subnet", gw)
	}
	var g struct {
		Gateway *string `json:"GATEWAY"`
	}
	if !resp.OK() {
		return "", &target.StatusError{Method: http.MethodGet, Path: path, Status: resp.Status}
	}
	if err := json.Unmarshal(resp.Body, &g); err != nil || g.Gateway == nil {
		return "", fmt.Errorf("reading the gateway: unexpected reply %.100q", resp.Body)
	}
	s.globalGateway = true
	if *g.Gateway == "" && s.cfg.Network.Gateway == "" {
		return s.leftoverGateway(), nil
	}
	return *g.Gateway, nil
}

// leftoverGateway is the first gateway stored with an interface the config
// names, or "".
func (s *networkSection) leftoverGateway() string {
	for _, c := range s.cfg.Network.Interfaces {
		if have := s.ifaces[c.Name].get("GATEWAY"); have != "" {
			return have
		}
	}
	return ""
}

// networkInSubnet reports whether ip is in the subnet of addr/mask.
func networkInSubnet(ip, addr, mask string) bool {
	i, a, m := net.ParseIP(ip).To4(), net.ParseIP(addr).To4(), net.ParseIP(mask).To4()
	if i == nil || a == nil || m == nil {
		return false
	}
	n := net.IPNet{IP: a.Mask(net.IPMask(m)), Mask: net.IPMask(m)}
	return n.Contains(i)
}

func (s *networkSection) Apply(ctx context.Context, t *target.Target, changes []state.Change) (After, error) {
	n := s.cfg.Network
	saved := map[string]bool{}
	applied := map[string]bool{}
	gatewayChanged, dnsChanged := false, false
	for _, c := range changes {
		switch {
		case c.Key == "network.gateway":
			gatewayChanged = true
		case strings.HasPrefix(c.Key, "network.dns"):
			dnsChanged = true
		default:
			for _, i := range n.Interfaces {
				prefix := "network." + i.Name + "."
				if !strings.HasPrefix(c.Key, prefix) {
					continue
				}
				if strings.TrimPrefix(c.Key, prefix) == "live" {
					applied[i.Name] = true
				} else {
					saved[i.Name] = true
				}
			}
		}
	}
	if gatewayChanged && !s.globalGateway && s.gatewayIface != "" {
		saved[s.gatewayIface] = true
	}
	if gatewayChanged && n.Gateway == "" {
		for _, i := range n.Interfaces {
			if s.ifaces[i.Name].get("GATEWAY") != "" {
				saved[i.Name] = true
			}
		}
	}
	for _, i := range n.Interfaces {
		switch {
		case saved[i.Name]:
			applied[i.Name] = true
		case dnsChanged:
			// Every interface's network file carries the DNS servers.
			applied[i.Name] = true
		case gatewayChanged && s.globalGateway && (i.Proto == "static" || n.Gateway == ""):
			// FPP routes the global gateway through every static interface.
			// Taking it away is applied through whatever the config names:
			// an apply rewrites every stored interface, and one the config
			// does not name may be the static one that had the route.
			applied[i.Name] = true
		}
	}

	// The order of the applies, the management interface last.
	var order []string
	for _, i := range n.Interfaces {
		if applied[i.Name] && i.Name != s.mgmt {
			order = append(order, i.Name)
		}
	}
	if s.mgmt != "" && applied[s.mgmt] {
		order = append(order, s.mgmt)
	}
	// DNS and the gateway take effect only through an interface's apply.
	if (dnsChanged || gatewayChanged) && len(order) == 0 {
		return None, errors.New("DNS or the gateway would be saved but never take effect: the config has no interface to apply them through")
	}

	// 1. Save. Nothing changes live yet.
	for _, i := range n.Interfaces {
		if !saved[i.Name] {
			continue
		}
		body := s.saveBody(i, s.ifaces[i.Name])
		if err := s.save(ctx, t, "/api/network/interface/"+i.Name, body); err != nil {
			return None, fmt.Errorf("saving interface %s: %w", i.Name, err)
		}
	}
	if dnsChanged {
		dns := map[string]string{"DNS1": "", "DNS2": ""}
		for j, d := range n.DNS {
			dns[fmt.Sprintf("DNS%d", j+1)] = d
		}
		if err := s.save(ctx, t, "/api/network/dns", dns); err != nil {
			return None, fmt.Errorf("saving DNS: %w", err)
		}
	}
	if gatewayChanged && s.globalGateway {
		if err := s.save(ctx, t, "/api/network/gateway", map[string]string{"GATEWAY": n.Gateway}); err != nil {
			return None, fmt.Errorf("saving the gateway: %w", err)
		}
	}

	// 2. Apply. No reply (a timeout, a dropped connection) says nothing, as
	// the reply may have nowhere to go; step 3 looks. A reply that refuses
	// is an error.
	for _, name := range order {
		path := "/api/network/interface/" + name + "/apply"
		actx, cancel := context.WithTimeout(ctx, s.applyTimeout)
		resp, err := t.API.Do(actx, http.MethodPost, path, nil, "")
		cancel()
		if err := ctx.Err(); err != nil {
			return None, err
		}
		if err == nil {
			if err := scrub(target.CheckOK(http.MethodPost, path, resp), s.secrets()...); err != nil {
				return None, fmt.Errorf("applying interface %s: %w", name, err)
			}
		}
	}

	// Follow the management address when its interface moves.
	if s.mgmt != "" && applied[s.mgmt] && !netLinkLocal(t.Address) {
		host := netHost(t.Address)
		for _, i := range n.Interfaces {
			if i.Name == s.mgmt && i.Proto == "static" && i.Address != host {
				t.API.SetAddress(i.Address)
				t.Address = i.Address
			}
		}
	}

	// 3. Verify: the controller answers where it should, then reads back
	// as the config says.
	if err := s.verify(ctx, t); err != nil {
		return None, err
	}
	return None, s.converged(ctx, t)
}

// save posts v to path, which must answer OK (target.CheckOK). Every PSK is
// masked out of the error, in case FPP or the transport repeats one.
func (s *networkSection) save(ctx context.Context, t *target.Target, path string, v any) error {
	return scrub(target.PostJSONOK(ctx, t.API, path, v), s.secrets()...)
}

// converged reads the network again and requires it to match the config.
func (s *networkSection) converged(ctx context.Context, t *target.Target) error {
	again := &networkSection{cfg: s.cfg}
	have, err := again.observe(ctx, t, true)
	if err != nil {
		return fmt.Errorf("after applying the network, reading it back: %w", err)
	}
	var keys []string
	for _, c := range state.Compare(s.Want(), have) {
		keys = append(keys, c.Key)
	}
	if len(keys) > 0 {
		return errors.New("after applying the network, the controller still differs from the config at " + strings.Join(keys, ", "))
	}
	return nil
}

// saveBody is the interface's stored config with the config's values laid
// over it, in the types FPP's own page sends.
func (s *networkSection) saveBody(i config.Interface, ifc *networkIface) map[string]any {
	body := map[string]any{}
	for k, raw := range ifc.stored {
		switch {
		case k == "PSKSet" || k == "BACKUPPSKSet":
			// Read-only markers (10.4); leaving PSK out keeps the saved one.
		case networkBoolKeys[k]:
			body[k] = networkTruth(target.Scalar(raw))
		default:
			body[k] = raw
		}
	}
	defaults := map[string]any{
		"ROUTEMETRIC": "0", "DHCPSERVER": false, "DHCPOFFSET": "100", "DHCPPOOLSIZE": "50", "IPFORWARDING": "0",
	}
	if strings.HasPrefix(i.Name, "wl") {
		for k, v := range map[string]any{
			"SSID": "", "HIDDEN": false, "WPA3": false,
			"BACKUPSSID": "", "BACKUPHIDDEN": false, "BACKUPWPA3": false,
		} {
			defaults[k] = v
		}
		if ifc.stored == nil {
			// Nothing is saved, so there is no PSK to keep.
			defaults["PSK"] = ""
			defaults["BACKUPPSK"] = ""
		}
	}
	for k, v := range defaults {
		if _, ok := body[k]; !ok {
			body[k] = v
		}
	}

	body["INTERFACE"] = i.Name
	body["PROTO"] = i.Proto
	if i.Proto == "static" {
		body["ADDRESS"] = i.Address
		body["NETMASK"] = i.Netmask
	} else {
		delete(body, "ADDRESS")
		delete(body, "NETMASK")
	}
	switch n := s.cfg.Network; {
	case !n.ManageGateway:
	case n.Gateway == "":
		if ifc.get("GATEWAY") != "" {
			body["GATEWAY"] = ""
		}
	case !s.globalGateway && s.gatewayIface == i.Name:
		body["GATEWAY"] = n.Gateway
	}
	if i.SSID != "" {
		body["SSID"] = i.SSID
		body["HIDDEN"] = i.Hidden
		body["WPA3"] = i.WPA3
		if i.PSK != "" {
			body["PSK"] = i.PSK
		}
	}
	if networkTruth(fmt.Sprint(body["DHCPSERVER"])) && len(ifc.leases) > 0 {
		body["Leases"] = ifc.leases
	}
	return body
}

// verify waits until the controller answers at each static address the
// config gives and at the management address, and each DHCP interface has
// a leased IPv4 address. Any address will not do: on 8.x the old static one
// stays for a moment after the apply answers.
func (s *networkSection) verify(ctx context.Context, t *target.Target) error {
	type check struct {
		what string
		ok   func(context.Context) bool
	}
	viaAPI := func(ctx context.Context) bool {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		resp, err := t.API.Do(c, http.MethodGet, "/api/fppd/version", nil, "")
		return err == nil && resp.Status == http.StatusOK
	}
	mgmtHost := netHost(t.Address)
	var checks []check
	mgmtChecked := false
	var dhcp []string
	for _, i := range s.cfg.Network.Interfaces {
		switch i.Proto {
		case "static":
			what := fmt.Sprintf("interface %s does not answer at %s", i.Name, i.Address)
			if i.Address == mgmtHost {
				checks = append(checks, check{what, viaAPI})
				mgmtChecked = true
				continue
			}
			url := "http://" + i.Address + "/api/fppd/version"
			checks = append(checks, check{what, func(ctx context.Context) bool {
				// A 401 is FPP answering with its UI password set; this
				// plain client has no credentials.
				st := netGetStatus(ctx, t, url)
				return st == http.StatusOK || st == http.StatusUnauthorized
			}})
		case "dhcp":
			dhcp = append(dhcp, i.Name)
		}
	}
	if !mgmtChecked {
		checks = append(checks, check{"the controller does not answer at " + t.Address, viaAPI})
	}
	for _, name := range dhcp {
		checks = append(checks, check{fmt.Sprintf("interface %s has no DHCP lease", name), func(ctx context.Context) bool {
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			all, _, err := networkList(c, t.API)
			return err == nil && all[name] != nil && all[name].leased
		}})
	}

	wait, cancel := context.WithTimeout(ctx, s.verifyTimeout)
	defer cancel()
	pending := checks
	err := target.WaitFor(wait, func(ctx context.Context) bool {
		var still []check
		for _, c := range pending {
			if !c.ok(ctx) {
				still = append(still, c)
			}
		}
		pending = still
		return len(pending) == 0
	}, s.poll)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var what []string
	for _, c := range pending {
		what = append(what, c.what)
	}
	return errors.New("after applying the network, " + strings.Join(what, "; "))
}

// secrets are every PSK the config gives and every one FPP returned (8.x),
// to be masked out of any error.
func (s *networkSection) secrets() []string {
	var out []string
	for _, i := range s.cfg.Network.Interfaces {
		out = append(out, i.PSK)
	}
	for _, ifc := range s.ifaces {
		out = append(out, ifc.get("PSK"), ifc.get("BACKUPPSK"))
	}
	return out
}
