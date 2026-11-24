# FPP API reference notes

What `fppcfg` needs from FPP's HTTP API, as far as it is known. Each item says
how it is known:

- **live**: observed on a BeagleBone Black running FPP 10.4, or on the
  controller and version named beside it.
- **source**: read in the FPP source at tag `10.4` (`www/api/`, `www/*.php`,
  `scripts/`), not exercised.

Nothing here has been checked on FPP older than 10.4 unless it says so.

## Identity and state

- `GET /api/system/info`: `HostName`, `HostDescription`, `Platform`, `Mode`,
  `Version`, `Branch`. (live)
- `GET /api/fppd/status`: player state. `channelOutputsEnabled` only means
  E1.31/DDP universe outputs are configured; it is false on a board that only
  drives pixel strings. (live)

## Settings

- `PUT /api/settings/<Name>` with the raw value as the body. (live)
- Older FPP writes any name to the settings file without checking it, so an
  unknown setting is stored, not rejected. (source, 8.5)
- `HostName` accepts only `[A-Za-z0-9-]`, so no underscores. A BeagleBone on
  10.4 stripped anything else; a Pi 4 on 10.4 refused the write with 400.
  (live)
- `GET /api/settings/<Name>` fills `value` from the definition's `default`
  when nothing is stored, as FPP's own type: `MultiSyncUnicast`
  (`"default": 1`) reads `"value": 1`, a number, on a remote that never set
  it. A checkbox with neither a stored value nor a default has no `value`
  key at all; fppd reads that as 0 (`getSettingInt`), so fppcfg reads it as
  `"0"`. (live, show player and remote 41 on 10.4, 2026-11-18; source,
  `src/settings.cpp`)
- MultiSync transport, three checkboxes, any combination, all
  `fppModes: ["player"]`: `MultiSyncMulticast` (to 239.70.80.80),
  `MultiSyncBroadcast`, `MultiSyncUnicast` (to every known remote). Only
  unicast has a default, and it is on, so off has to be stored as `"0"`.
  None asks for a restart: fppd re-reads the settings file when the UI writes
  it and rebuilds its destinations from a listener on each of the three.
  (source, `www/settings.json`, `src/MultiSync.cpp`, 10.4) The show player was
  switched to multicast only in the UI with fppd left running and sent
  multicast straight away. (live, 10.4, 2026-11-18) On the first boots after
  an FPPOS upgrade or a settings restore, a player with MultiSync on and none
  of the three stored gets multicast written, and an absent
  `MultiSyncUnicast` is stored as `"0"`. A fresh image does neither, so it
  sends unicast. (source, `checkConfigMigrations` in
  `src/boot/FPPINIT_Config.cpp`)
- On 8.4.1 there is no `MultiSyncUnicast`, so fppcfg notes it as not on that
  FPP and skips it. `MultiSyncMulticast` defaults on there, and both
  multicast and broadcast ask for a quick restart. (source, `www/settings.json`
  at tag 8.4.1)

## Network

- `POST /api/network/interface/<iface>` with a JSON object: `INTERFACE`,
  `PROTO`, `ADDRESS`, `NETMASK`, `GATEWAY`, `ROUTEMETRIC`, the DHCP server
  fields, and for WiFi `SSID`, `PSK`, `HIDDEN`, `WPA3` and the backup network
  fields. (live, 8.4.1)
- `POST /api/network/dns` with `{"DNS1":..., "DNS2":...}`. (live, 8.4.1)
- `POST /api/network/gateway` with `{"GATEWAY":...}`: the gateway is also
  settable on its own. (live) An empty value clears it. (source,
  `www/api/controllers/network.php`, 10.4)
- The global gateway is written into the network file of static interfaces
  only; an interface on DHCP never gets it. Stored DNS servers are written
  into every interface's file, DHCP ones included, where they sit beside the
  servers the lease gives. (source, `src/boot/FPPINIT_Network.cpp`, 10.4)
- When the global gateway is empty, 10.4 looks through the interface files
  for an old per-interface `GATEWAY`, makes the first one found the global
  gateway and removes it from that file. So clearing the global gateway does
  not last while an interface still stores one. (source, same file)
- Nothing takes effect until `POST /api/network/interface/<iface>/apply`. The
  call times out when the address changes, because the reply has nowhere to go.
  A restored static config stays unapplied, and the board stays on DHCP, until
  apply runs. (live)
- IPv6 link-local keeps working alongside a static IPv4 config, and the API
  and SSH answer on it. The address is derived from the interface MAC, so it
  changes when the board is swapped. FPP only loses it when the kernel is
  booted with IPv6 disabled. (live, source)

## Setup gate

Newer FPP redirects every UI page to `initialSetup.php` until all of the
following hold. The API keeps answering while the gate is up, so a controller
that answers the API is not necessarily set up. (live)

1. `initialSetup-02` is `1`.
2. The settings file has `LegalJurisdiction`, `statsPublish`,
   `ShareCrashData`, `FetchVendorLogos` and `SendVendorSerial`.
3. `GET /api/privacy/consent` returns `"shortfall": ""`.

The consent record:

- `POST /api/privacy/consent` with
  `{"via": "wizard", "date": "<ISO 8601>", "settings": {...}}`, where
  `settings` holds the values of `statsPublish`, `FetchVendorLogos`,
  `SendVendorSerial` and `ShareCrashData`. `via` must be `wizard` or
  `settings`. FPP adds the version, a hash of the disclosure text and the
  device UUID itself. (source)
- `shortfall` is one of `absent`, `incomplete`, `stale-version`,
  `other-device` or empty. A record restored from another board's backup is
  `other-device`. (source)
- The response also lists `drift`: settings whose live value no longer matches
  the recorded one. (source)
- The consent API exists in tags 10.2 to 10.4 and not in 10.0. Probe for it
  (a JSON reply with a `shortfall` key) instead of comparing versions. (source)

## SSH keys

- `POST /api/configfile/authorized_keys` with the file as the body stores it
  as `media/config/authorized_keys` and runs `scripts/installSSHKeys`, which
  copies it over `/root/.ssh/authorized_keys` and
  `/home/fpp/.ssh/authorized_keys`. Whole-file replace. (source)
- `GET /api/configfile/authorized_keys` returns the stored copy, or 404 when
  there is none. (live)
- The stored copy is not the truth: a key placed directly in
  `/home/fpp/.ssh/authorized_keys` shows up neither here nor on the settings
  page, which reads root's file. Reading the real files needs SSH. (live)

## Backups

- FPP writes a JSON backup under `media/config/backups/` on each config
  change. (live)
- `curl -F btnDownloadConfig=1 -F backuparea=all <host>/backup.php` downloads
  a full backup. (live)

## FPP 8.4.2

Read from the show player (Raspberry Pi 5, FPP 8.4.2), the only controller up
on 2026-11-11. GETs only; nothing was written. (all live, 8.4.2)

- `GET /api/system/info`, `/api/fppd/status`, `/api/network/interface`,
  `/api/network/interface/eth0` and `/api/network/dns` all answer as on 10.4.
- Idle status is `"status": 0, "status_name": "idle"`, with `mode_name`
  `player`. The playing value was not seen.
- `GET /api/settings/<Name>` returns the setting's definition with its
  `value`. The definition says what a change needs: `fppMode` has
  `"restart": 2`, `HostName` has `"reboot": 1`, `AudioOutput` has
  `"reboot": 1`. Settings `restartFlag` and `rebootFlag` exist and read `0`.
  Who sets those flags on a write, and whether 10.4 is the same, is not
  checked.
- `GET /api/settings` returns `settingGroups`, `attributes` and `settings`
  (about 70 KB), not a flat name-to-value map.
- `GET /api/privacy/consent` returns Limonade's HTML error page, not JSON, so
  the consent probe correctly finds no consent API.
- `GET /api/configfile/authorized_keys` returned 404 while nine keys were
  installed, so no stored copy says nothing about the real files.
- `POST /api/configfile/authorized_keys` with the file as the body (as
  `text/plain`) answers `{"Status":"OK","Message":""}`, stores the copy and
  replaces both `/root/.ssh/authorized_keys` and
  `/home/fpp/.ssh/authorized_keys` whole, as on 10.4. A `cert-authority` line
  works: login by certificate succeeded straight after. The install also sets
  every file in both `.ssh` directories to mode 600.
- `settings-system.php` embeds root's `authorized_keys` in its `sshKeys`
  textarea, so root's real file can be read over HTTP. fpp's cannot.
- `passwordEnable` is a setting (`0`/`1`) with child settings `password` and
  `passwordVerify`. `AudioOutput` is the sound device, a card index, with the
  choices at `GET /api/settings/AudioOutput/options`.

## Network on FPP 8.4.1

Exercised on a Pi 4 running 8.4.1, reached over ethernet and WiFi at once.
(all live, 8.4.1)

- `POST /api/network/interface/eth0` with a static or DHCP config, then
  `POST /api/network/interface/eth0/apply`, moves the address within seconds
  and needs no reboot. Static to DHCP and back both work.
- Unlike 10.4, apply answers: `200` after about two seconds, sent from the old
  address, with fppinit's log lines in `output`. The old address then stops
  answering and the new one starts. A tool cannot rely on a timeout to mean
  "the address changed"; it has to look at the new address either way.
- Applying one interface leaves the other alone: eth0 stayed up through a
  wlan0 apply, and the reverse. That holds only while the other's stored
  config is already in effect: the apply runs `fppinit setupNetwork
  <interface>`, and `setupNetwork()` takes no argument and brings up every
  stored interface. (source, 8.4.2) So an apply on any one interface also
  moves another whose saved config differs from what is running. Seen live
  (8.4.1, 2026-11-14): with a new eth0 address and new DNS saved, the apply
  on `wlan0` moved eth0 about thirteen seconds into the push, and WiFi stopped
  answering for about five seconds around it. The apply on `eth0` that
  followed, sent to the old address, got no answer until its timeout.
- After a reboot the API answered on eth0 about 20 seconds before wlan0 had
  associated and taken its lease; in between wlan0 shows no IPv4 address.
  (live, 8.4.1)
- WiFi takes the same POST with `PROTO`, `SSID`, `PSK`, `HIDDEN`, `WPA3` and
  the backup fields, then apply on `wlan0`; it associated and took a DHCP
  lease in about ten seconds. `GET /api/network/interface/wlan0` returns the
  PSK in plaintext, and booleans sent as `false` read back as `""`.
- `GET /api/network/interface` shows each interface's stored `config` beside
  its live addresses, so a stored but unapplied config is visible as a
  mismatch between the two. An interface never configured has `config: null`.
- `POST /api/network/dns` works and reads back; before any write the GET
  answers `{"status": "Not Configured"}`.
- `/api/network/gateway` does not exist (404). On 8.x the gateway can only be
  set as part of an interface.

## Restart and reboot

Read at tags `8.4.2` and `10.4`; the same at both unless noted. (all source)

- The API does not say that a write needs a restart or reboot. The browser
  does that work: it reads `restart` and `reboot` from the setting's
  definition and writes the `restartFlag` and `rebootFlag` settings itself.
  A tool must do the same from `GET /api/settings/<Name>`.
- `restart` is `0` (none), `1` (full restart) or `2` (quick restart is
  enough). `reboot` is `0` or `1`.
- `GET /api/system/fppd/restart` restarts fppd; with `?quick=1` it sends fppd
  a restart command instead of stopping and starting it. `GET
  /api/system/reboot` reboots after one second. 10.4 also accepts POST for
  both and has `GET /api/system/fppd/restartStatus`; 8.4.2 has GET only.
- The flags only drive the UI banner. A tool that restarts by itself should
  leave them, or clear a flag someone else set, by writing `0`.
- Writing `fppMode`, whose definition has `restart: 2`, left `restartFlag`
  at `0`. (live, 8.4.1)
- A quick restart answers at once; a full one takes about five seconds to
  answer. For a second or two afterwards `/api/fppd/status` returns something
  that is not JSON, then `uptime` (fppd's, not the board's) starts again from
  zero. POST to the restart call and `restartStatus` are both 404.
  (live, 8.4.1)
- An unknown setting name is stored and read back, not rejected.
  (live, 8.4.1)
- `GET /api/settings/<Name>` for a name that is neither defined nor stored
  answers `[]` with 200 (10.4 answers `{"value": ...}`). So do
  `LegalJurisdiction` and `initialSetup-02` on a fresh board; the four
  privacy settings are defined. (live, 8.4.1)
- The setup gate exists: every page refreshes to `initialSetup.php` until
  `initialSetup-02` is stored as `1`, though that setting has no definition.
  Nothing else holds it up, and there is no consent API. (source, 8.4.2;
  live, 8.4.1: a fresh board's gate came down on that one write)

## Playing

- `status` is `0` (`idle`) when nothing plays; anything else (`playing`,
  `paused`, the `stopping` states, `testing` by name) means busy. (source)
- A remote reports `status: 1`, `playing`, while a synced sequence or media
  file runs. (source)
- A remote that is fed DDP, as the show's are, is not running a sequence, so
  its `status` stays idle. (source)
- `bridging` does not mean data is arriving now. fppd sets it on the first
  channel-data packet and never clears it at 8.4.2; 10.4 clears it only when
  the inputs are reloaded. So it reads "has received data since fppd
  started". (source)
- `GET /api/fppd/e131stats` reports bytes and packets received per input,
  DDP included. Two reads a moment apart would show whether data is arriving
  now. (source; the call answers on 8.4.1, live)
- A Pi 4 on 8.4.1 with no outputs configured ignored DDP frames sent to port
  4048, in player and in remote mode: the packets reached the board, but
  `bridging` stayed false and `e131stats` stayed empty. Why is not known, so
  neither field has been seen to change. (live, 8.4.1)

## Audio

- `AudioOutput` is the sound device, a card index; a change needs a reboot on
  8.4.2. (live, 8.4.2)
- `POST /api/system/volume` with `{"volume": 0-100}` sets the volume, and
  `GET` reads it; `/api/fppd/status` shows it as `volume`. (live, 8.4.1)

## Passwords

Read at tags `8.4.2` and `10.4`, and exercised on a Pi 4 running 8.4.1.
(source; live, 8.4.1; for 10.4 see "FPP 10.4 on a Pi 4")

- UI password: write the setting `password`, then `passwordEnable` to `1`.
  FPP stores a hash for two users, `admin` and `fpp`, in
  `media/config/.htpasswd`. An empty password becomes `falcon`.
- With it enabled, Apache asks for Basic auth on everything under `www/`,
  the API included, except from the controller itself. So the tool needs the
  password to reach a protected controller. Without it every request is 401
  with `WWW-Authenticate: Basic realm="Falcon Player"`; either user works.
  Writing `passwordEnable` to `0` with credentials opens it again.
- `GET /api/settings/password` returns the password in plaintext, so a diff
  can compare it but must not print it.
- The OS password takes effect as soon as `osPassword` is written, before
  `osPasswordEnable` is touched.
- OS password of the `fpp` account: write `osPassword`, then
  `osPasswordEnable` to `1`. Setting `osPasswordEnable` to `0` resets the
  account's password to `falcon`.
- The settings file keeps both passwords in plaintext.

## Output processors

- `GET /api/channel/output/processors` returns
  `{"status": "OK", "outputProcessors": [...]}`. (live, 8.4.2)
- `POST` to the same path replaces the whole file with the body and returns
  the result. The same code at `8.4.2` and `10.4`. (source)
- The POST works as described and answers with the saved list.
  (live, 8.4.1)
- After a save, 8.4.2's page asks for a quick restart (`restart` 2); 10.4's
  page asks for nothing, and 10.4's fppd loads the new list at once without
  a restart (its log shows the processor loading at the POST).
  (source; live, 10.4)

## Proxies

Read at tags `8.4.2` and `10.4` (`www/api/controllers/proxies.php`). (source,
unless marked)

- `GET /api/proxies` returns `[{"host": ..., "description": ...}]`, read
  back from `media/config/proxy-config.conf` (`media/config/proxies` on
  8.4.2), the Apache rewrite rules behind `/proxy/<host>/`. It answered `[]`
  on the show player with none set. (live, 10.4)
- On 10.4 the list also has the addresses FPP's own DHCP server has leased,
  each with `"dhcp": true`. One not in the file yet also has
  `"pending": true`; every other entry has `"pending": false`.
- `POST /api/proxies` with the whole list as JSON replaces the file and
  answers with what it saved. On 10.4 it drops a host that is not an IPv4
  address or `[A-Za-z0-9.-]+`, adds the current DHCP leases to what it
  writes, reloads Apache and writes a config backup. An empty list (or none
  valid on 10.4) is refused with 400.
- Setting one host on the show player worked as above: `/proxy/<host>`
  answers 301 to `/proxy/<host>/`, which reached the other controller's API
  straight away, and the playing fppd carried on. (live, 10.4, 2026-11-20)
- A description is kept as a `# D:` comment line above the host's rules.
  10.4 strips tags from it, turns line breaks into spaces and cuts it at 128
  bytes. Reading back, a line holding `http://` is taken for a rule, so a
  description with it is misread.
- `DELETE /api/proxies` (10.4 only) writes an empty file and reloads Apache.
  That drops the stored DHCP lease entries too. 8.4.2 has no such route, and
  its `DELETE /api/proxies/<host>` empties the whole list whatever the host
  (it loops over an undefined variable).
- `POST` and `DELETE /api/proxies/<host>` add or remove one host.

## FPP 10.4 on a Pi 4

Exercised on a Pi 4 booted from a fresh FPP 10.4 image (OS `v2026-11`, Debian
13), on the bench network. (all live, 10.4, unless marked)

Same as 8.4.x:

- `POST /api/configfile/authorized_keys` replaces both accounts' files whole
  (mode 600) and certificate login works straight after. Before any upload the
  GET answers with Limonade's error page.
- Setting definitions carry `restart` and `reboot`; writing `fppMode` or
  `HostName` leaves `restartFlag` and `rebootFlag` at `0`.
- An unknown setting name is stored and read back, not rejected. The API has
  no call to delete a setting.
- UI password: `password` then `passwordEnable` gives 401 without Basic auth,
  200 for `admin` and `fpp`, 200 from the controller itself. The password
  alone, before `passwordEnable`, locks nothing.
- OS password: the `fpp` account's hash changes as soon as `osPassword` is
  written, and `osPasswordEnable` `0` puts it back to `falcon`.
- Both passwords are kept in plaintext in the settings file.
- Volume and output processors behave as on 8.4.1.
- Applying one interface leaves the other up. WiFi associates and takes a
  lease within about fifteen seconds of apply.

Different from 8.4.x:

- Secrets are not readable. `GET /api/settings/password` and `osPassword`
  return the definition with no `value`, and an interface GET returns
  `PSKSet: true` in place of `PSK`. So a diff on 10.4 cannot compare them
  directly. The UI password can be checked by trying it; the OS password
  only by SSH; the PSK not at all.
- A WiFi POST that leaves out `PSK` keeps the saved one. (source)
- `apply` on the interface the request came in on times out (30 s seen) and
  the board answers at the new address within seconds. Applied from the other
  interface it answers 200 in about five seconds.
- `/api/network/gateway` exists (GET and POST; empty clears it). The gateway
  is global, and an interface's stored `config` has no `GATEWAY`.
- A hardware interface's config cannot be deleted (`DELETE` answers 409; source).
  Saving `wlan0` with an empty SSID and applying drops its address, but the
  radio stays associated until the next boot.
- `HostName` changes the OS hostname at once; the definition still says
  `reboot: 1`. An invalid name is refused with 400.
- `AudioOutput` holds the card's ALSA name (`Headphones`, `vc4hdmi0`), not an
  index. Choices are at `/api/settings/AudioOutput/options`.
- Restart and reboot accept POST as well as GET. A quick restart answers in
  50 ms, a full one in about three seconds; `/api/fppd/status` answers 503
  for a second or so afterwards. `GET /api/system/reboot` had the API back in
  about 33 seconds.
- A quick restart keeps the fppd process: `ps` shows the same elapsed time
  afterwards. It is an in-process restart (`SendCommand('restart')` in
  `www/api/controllers/system.php`), and `uptimeTotalSeconds` starts again
  from zero, so a drop in uptime shows that it happened. (live, 10.4)

Setup gate, taken down step by step:

- The gate is not an HTTP redirect: pages answer 200 with a
  `<meta http-equiv='refresh'>` to `initialSetup.php`.
- On a fresh board the four privacy settings read as their defaults
  (`statsPublish` `Banner`, `ShareCrashData` `3`, the vendor two `1`) though
  the settings file does not have them. A GET cannot tell "unset" from
  "default"; `drift` in the consent reply lists all four.
- `initialSetup-02` alone left the gate up, and so did adding
  `LegalJurisdiction` (one of `EU`, `UK`, `US`, `CA`, `AU`, `OTHER`) and the
  four privacy settings. The consent POST then took it down. The record is
  stored as the setting `privacyConsent`.
- A `via` other than `wizard` or `settings` answers 200 with
  `"Status": "Error"`.
- Changing a recorded setting afterwards shows it in `drift`, leaves
  `shortfall` empty and does not bring the gate back.

## Still to find out

- How to switch the test board between its uSD (8.4.1) and USB drive (10.4)
  without hands. With the uSD's `start4.elf` renamed the board did not fall
  through to USB; with the uSD pulled it boots USB.

Not needed: what `bridging` and `e131stats` show under DDP. The tool ignores
DDP when deciding whether a controller is playing.
