// Shared by every controller config in this directory. fppcfg loads every
// .cue file beside the config it is given, so nothing here needs importing.
//
// CUE unifies rather than overrides: a station that sets a field to another
// value is an error, unless the value here is marked as a default with "*".
// A station can replace a default with another value, or with null to leave
// that part of the controller alone.

settings: {
	fppMode:               "remote"
	MultiSyncEnabled:      true
	Service_rsync:         true
	bridgeDataPriority:    "Prioritize Sequence"
	blankBetweenSequences: true
	Locale:                "USA"
	TimeZone:              "America/Los_Angeles"
	ntpServer:             "time.google.com"
	uiLevel:               *1 | int // a station may raise it
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

// A station on DHCP says gateway: "dhcp" and dns: "dhcp" to use what its
// lease gives and store neither.
network: {
	gateway: *"192.168.1.1" | _
	dns:     *["8.8.8.8", "8.8.4.4"] | _
}
