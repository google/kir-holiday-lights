// Shared by every controller config in this directory. fppcfg loads every
// .cue file beside the config it is given, so nothing here needs importing.
//
// CUE unifies rather than overrides: a controller that sets a field to
// another value is an error, unless the value here is marked as a default
// with "*". A default can be replaced by another value, or by null to leave
// that part of the controller alone.
settings: {
	// Every controller is a remote except the show player (192-…), which sets
	// both of these. A remote ignores MultiSyncEnabled, but one switched to
	// player mode for local testing would otherwise start the show on every
	// remote.
	fppMode:               *"remote" | "player"
	MultiSyncEnabled:      *false | true
	Service_rsync:         true
	bridgeDataPriority:    "Prioritize Sequence"
	blankBetweenSequences: true
	Locale:                "USA"
	TimeZone:              "America/Los_Angeles"
	ntpServer:             "time.google.com"
	uiLevel:               *1 | int
	Latitude:              47.6784
	Longitude:             -122.1857
}

// The setup gate. fppcfg records privacy consent for exactly these values.
setup: {
	LegalJurisdiction: "US"
	statsPublish:      "Disabled"
	ShareCrashData:    0
	FetchVendorLogos:  false
	SendVendorSerial:  false
}

// The gateway and DNS servers of the show network. A controller on DHCP
// replaces both with "dhcp": it stores neither and uses what its lease gives.
network: {
	gateway: *"192.168.1.1" | _
	dns:     *["8.8.8.8", "8.8.4.4"] | _
}

// The show WiFi, for a controller that has a wlan0 (the gift boxes): it
// gives only its address. The password comes from secrets.cue; without it
// such a config does not load, so WiFi is never written without one.
network: interfaces: wlan0?: {
	proto:   "static"
	netmask: "255.255.255.0"
	ssid:    "KIR Lights"
	psk!:    string
}

// Who may log in as root and fpp over SSH: the whole authorized_keys file,
// from secrets.cue; a key not listed there is removed. Without it no config
// loads, so the keys are never written without one.
sshKeys!: [...]
