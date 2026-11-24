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

// The schema for one controller's config. Load unifies the config files with
// #Controller, which is closed: a field not listed here is an error.
//
// Settings are named by FPP's own setting names. Adding a managed setting is
// one line in #Settings; the order there is the order fppcfg shows them in.
//
// An optional value may also be null, which means "not managed", the same as
// leaving it out. A controller uses it to drop a value a shared file gives
// as a default: volume: *70 | _ there, volume: null here.

import (
	"list"
	"net"
	"strings"
)

#Flag: bool
#Text: string
#IPv4: string & net.IPv4
#IP:   string & net.IP

// Secrets must not be empty: FPP turns an empty password into "falcon".
#Secret: string & !=""

#HostName: string & =~"^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$" & strings.MaxRunes(30)

// One authorized_keys line: something on it, and no line break inside.
#KeyLine: string & =~#"^[^\r\n]*\S[^\r\n]*$"#

#Controller: {
	version?: #Text | null
	// Management address: IPv4 or IPv6. Write link-local without a zone
	// ("fe80::ba27:ebff:fe12:3456"): the tool finds the interface here that
	// reaches it.
	address?: #IP | null

	settings?: #Settings
	setup!:    #Setup

	volume?: (int & >=0 & <=100) | null

	passwords?: {
		ui?: #Secret | null
		os?: #Secret | null
	}

	sshKeys?: [...#KeyLine] | null

	// FPP's own output processor objects, passed through as they are.
	outputProcessors?: [...{type!: string, ...}] | null

	// The hosts FPP proxies at /proxy/<host>/, replaced whole.
	proxies?: [...#Proxy] | null

	network?: #Network
}

// One proxied host. FPP keeps the description as a comment in its Apache
// config: it strips tags and line breaks, cuts it at 128 bytes, and takes a
// line holding "http://" for a rule. Printable ASCII without "<" or
// "http://" reads back as written.
#Proxy: {
	host!:        string & =~"^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$"
	description?: string & =~"^[ -;=-~]{0,128}$" & !~"http://"
}

#Settings: {
	HostDescription?:       #Text | null
	HostName?:              #HostName | null
	fppMode?:               "player" | "remote" | null
	MultiSyncEnabled?:      #Flag | null
	// How the player sends MultiSync; any combination. FPP 10 turns unicast
	// on by default, so leaving it out does not mean off.
	MultiSyncMulticast?:    #Flag | null
	MultiSyncBroadcast?:    #Flag | null
	MultiSyncUnicast?:      #Flag | null
	Service_rsync?:         #Flag | null
	bridgeDataPriority?:    #Text | null
	blankBetweenSequences?: #Flag | null
	Locale?:                #Text | null
	TimeZone?:              #Text | null
	ntpServer?:             #Text | null
	uiLevel?:               (int & >=0 & <=3) | null
	Latitude?:              number | null
	Longitude?:             number | null
	// A card index on FPP 8.x, an ALSA device name on 10.x.
	AudioOutput?: string | int | null
}

// The setup gate. All five are required, so consent is only ever recorded
// for values the config states.
#Setup: {
	LegalJurisdiction!: "EU" | "UK" | "US" | "CA" | "AU" | "OTHER"
	statsPublish!:      "Enabled" | "Disabled" | "Banner"
	ShareCrashData!:    -1 | 0 | 1 | 2 | 3
	FetchVendorLogos!:  #Flag
	SendVendorSerial!:  #Flag
}

#Network: {
	// A null interface is one the controller does not manage.
	interfaces?: [Name=string]: #Interface | null
	// "dhcp" means none of our own: the controller keeps no static gateway
	// (or DNS servers) and uses what its DHCP leases give. Unlike null, the
	// tool then removes any that are stored.
	gateway?: #IP | "dhcp" | null
	dns?:     ([...#IP] & list.MaxItems(2)) | "dhcp" | null
}

#Interface: {
	proto!:   "static" | "dhcp"
	address?: #IPv4
	netmask?: #IPv4
	ssid?:    string & !=""
	psk?:     #Secret
	hidden?:  #Flag
	wpa3?:    #Flag

	if proto == "static" {
		address!: _
		netmask!: _
	}
}
