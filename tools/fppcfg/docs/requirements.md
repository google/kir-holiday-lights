# fppcfg requirements

`fppcfg` brings one FPP controller to the state described by one config file,
and shows the difference first. This document records what it must do; it
is not a design.

## Scope

- Manages FPP controllers only, the show player included. Devices that are not
  FPP (the Falcon F4V3, the floods) have no config and are never touched.
- Manages:
  - FPP settings: description, hostname, mode, MultiSync, rsync, bridge
    priority, blanking, locale, time zone, NTP, UI level, location, the privacy
    settings.
  - Network: ethernet, WiFi, gateway, DNS.
  - The setup gate on newer FPP: jurisdiction, privacy consent record, setup
    complete flag. A pushed controller is ready to use with no wizard step.
  - Audio settings (sound device, volume).
  - Passwords (FPP web UI, the `fpp` OS account), when the config sets them.
  - SSH `authorized_keys` for FPP's `root` and `fpp` accounts, through the
    copy FPP stores and installs.
  - Output processors (such as masking a bad pixel), as one list replaced
    whole. FPP reads and writes them as a single file, the same way on 8.4.2
    and 10.4, which met the "only if trivial" test.
  - Proxies: the hosts whose web UI FPP serves at `/proxy/<host>/`, as one
    list replaced whole, each with an optional description. xLights does not
    set them on FPP 10.4. Hosts FPP proxies on its own, because its DHCP
    server leased them an address, are FPP's: shown only when the config
    names the host, and kept when the list is written.
- Does not manage: channel outputs, channels, playlists, media, sequences
  (xLights owns these), plugins, FPP upgrades or downgrades.
- Does not manage anything that is not FPP. Monitoring (the Prometheus
  exporters the gateway scrapes) is set up by a separate tool or script.
- Uses FPP's HTTP API only. It does not log in to the controller over SSH.
- Later, low priority: the show player's schedule.

## Config

- One config per controller, in CUE, so shared defaults and templates can be
  factored out.
- Secrets (WiFi PSK, passwords) are plaintext in a common template file that is
  gitignored.
- FPP settings are named by FPP's own setting names. The schema is closed: a
  setting it does not list is a config error.
- A controller's config can take back what a shared template gives it,
  provided the template marks the value as a default (`*"value" | _`).
  - `null` means "not managed", exactly as if nothing had been written: the
    tool neither shows nor changes that part of the controller. These can be
    `null`: the version, the address, each setting, the volume, each
    password, the SSH keys, the output processors, the proxies, the gateway,
    DNS, and a whole interface. The five setup-gate values are required and
    cannot be, and nor can a single field inside an interface.
  - The gateway and DNS also take the value `"dhcp"`: the controller keeps no
    gateway (or DNS servers) of its own and uses what its DHCP leases give.
    This is a managed state, not an absence: `push` removes a stored gateway
    or stored DNS servers, and applies the interfaces the config names so the
    change takes effect. A gateway stored with an interface the config does
    not name is out of reach; FPP 10.4 takes it up again, and `push` then
    reports that the gateway still differs.
- A config that sets DNS or the gateway also names at least one interface:
  FPP brings those into effect only when an interface is applied.
- Every config gives the jurisdiction and the four privacy settings, in
  practice from a shared template. The tool records consent on newer FPP, and
  does so only for values the config states.
- A config may set a management address, IPv4 or IPv6. Link-local IPv6 is the
  usual choice: it is stable and FPP does not manage it. Without one, the tool
  uses the controller's configured IPv4 address. A link-local address is
  written without a zone, since the zone names an interface on the operator's
  machine, not the board's. The tool finds the interface that reaches it, and
  `--at fe80::…%IFACE` names one when that is ambiguous.
- The tool has no dependency on xLights files or on this repository's layout.
  It works installed anywhere, given a config file.

## Commands

    fppcfg ./station.cfg diff [--full] [--at ADDRESS] [--password OLD]
    fppcfg ./station.cfg push [--force] [--at ADDRESS] [--password OLD]

- `diff` never writes. It compares the config with what the API reports. For
  SSH keys that is the copy FPP stores; a controller whose keys were put in
  place another way shows them as missing.
- Secrets are never printed.
  - Passwords are not compared, and `diff` does not show them; it shows only
    whether each is enabled. Every `push` writes the passwords the config
    sets, and the API accepting the write is the only check.
  - The WiFi PSK is not compared either: FPP 10.4 reports only whether one
    is set. `diff` shows that. `push` writes the PSK when none is set, and
    whenever it rewrites the interface for another change. A change to the
    PSK alone is not seen. Rewriting it on every push would re-apply WiFi
    every time.
- Settings the config does not mention are left alone and not shown.
  `diff --full` also lists every setting the tool can manage that the config
  leaves unset, with its current value.
- The diff program comes from `FPPCFG_DIFF`, or failing that `DIFF` (default
  `diff -u`), so
  `colordiff`, `delta` and the like work. The tool hands it current and desired
  state as two files. `diff` exits with the diff program's status (0 when
  there is nothing to compare). Errors exit with 2, as `diff`'s do.
- `push` prints the diff and applies it without asking.
  - It refuses while the controller is playing, unless `--force`. Playing
    means its own status is not idle. Channel data arriving from elsewhere
    (DDP into a remote) is ignored: the tool does not look for it.
  - When a change needs an fppd restart or a reboot, it does that itself.
  - An address change goes last, and the tool confirms the controller answers
    at the new address before reporting success.
  - `authorized_keys` is replaced whole: a key that is not in the config is
    removed.
  - It takes no backup of its own; FPP saves one on each config change.
- `--at ADDRESS` overrides where to reach the controller for this run, for a
  new or replacement board still at a temporary address.
- A controller with a UI password asks for it on the API too. The tool logs in
  with the password in the config and, if that is refused, tries without one
  (a fresh board). If neither works it stops and says so. `--password OLD`
  gives the current password for the one run that changes it.

## Versions

- Controllers run mixed FPP versions, the oldest being 8.x. The tool finds out
  what each
  one supports and adapts, skipping steps that controller's FPP does not have.
- A config may state the FPP version the controller is expected to run. `diff`
  reports a mismatch; `push` does nothing about it.

## Implementation

- Go: fast startup, and a static binary that is easy to distribute.
- The config-loading code is an importable package, so other tools can read
  the same configs (for example to generate the gateway's Prometheus target
  files). `fppcfg` itself has no export command.
- It can be tested against recorded controller responses, without a live
  controller.

## Open

Nothing at present.
