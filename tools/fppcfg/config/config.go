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

// Package config loads fppcfg's per-controller configs. It is importable so
// other tools (the gateway's monitoring target lists, for one) can read the
// same files.
//
// A config is a CUE file, by convention named "<controller>.cfg". Every
// ".cue" file in the same directory is loaded with it, so shared defaults
// and the gitignored secrets.cue apply to every controller in a directory
// without each config naming them. The result must satisfy the schema in
// schema.cue, which is closed: an unknown field is an error.
//
// An optional value may be null, which is the same as leaving it out. It is
// how one controller drops a value that a shared file gives as a default.
package config

// Controller is one controller's config, as the tool uses it. Zero values
// mean "not managed": the tool leaves that part of the controller alone.
type Controller struct {
	// Version is the FPP version the controller is expected to run, or "".
	Version string
	// Address is the management address (IPv4 or IPv6), or "". A link-local
	// address normally has no zone; the command adds the operator's
	// interface.
	Address string

	// Settings are the ordinary FPP settings the config sets, by FPP's own
	// names, in schema order. Values are in FPP's string form ("1" for
	// true, numbers formatted plainly).
	Settings []Setting

	// Setup is the setup gate's settings, in this fixed order:
	// LegalJurisdiction, statsPublish, ShareCrashData, FetchVendorLogos,
	// SendVendorSerial. The schema requires all five.
	Setup []Setting

	// Volume is 0-100, or nil.
	Volume *int

	// Passwords are secrets; "" means not managed.
	UIPassword string
	OSPassword string

	// SSHKeys is the whole authorized_keys file for root and fpp, one line
	// per key. ManageKeys is false when the config does not set it.
	SSHKeys    []string
	ManageKeys bool

	// OutputProcessors is the whole list, each item as canonical JSON (keys
	// sorted, no spaces). ManageProcessors is false when the config does not
	// set it; an empty list with ManageProcessors true means "none".
	OutputProcessors []string
	ManageProcessors bool

	// Proxies is the whole list of hosts FPP proxies. ManageProxies is false
	// when the config does not set it; an empty list with ManageProxies true
	// means "none".
	Proxies       []Proxy
	ManageProxies bool

	Network Network
}

// Setting is one FPP setting.
type Setting struct {
	Name  string
	Value string
}

// Proxy is one host FPP proxies at /proxy/<host>/.
type Proxy struct {
	Host        string
	Description string // "" for none
}

// Network is the controller's network config. Interfaces the config does not
// name are left alone.
type Network struct {
	Interfaces []Interface // sorted by name
	// Gateway is the default gateway. ManageGateway is false when the config
	// does not set it. ManageGateway with Gateway "" is the config's "dhcp":
	// the controller is to store no gateway of its own.
	Gateway       string
	ManageGateway bool
	// DNS is up to two servers. ManageDNS is false when the config does not
	// set it. ManageDNS with no servers is the config's "dhcp" (or an empty
	// list): the controller is to store none of its own.
	DNS       []string
	ManageDNS bool
}

// Interface is one network interface.
type Interface struct {
	Name    string // "eth0", "wlan0"
	Proto   string // "static" or "dhcp"
	Address string // static only
	Netmask string // static only, dotted quad
	// WiFi only; SSID "" means not a WiFi interface.
	SSID   string
	PSK    string // secret
	Hidden bool
	WPA3   bool
}
