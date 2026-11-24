# fppcfg design

How `fppcfg` is built. What it must do is in `requirements.md`; what FPP's API
does is in `fpp-api.md`.

## Shape

A short, fixed list of sections behind one interface, with FPP settings handled
as data and not as code.

Two other shapes, and why not:

- One function per area, each reading, comparing and writing. Simple to start, but `diff` and `push` become separate code
  paths that drift, and a new setting touches several functions.
- A general resource engine with a planner, a dependency graph and a type per
  parameter. `diff` and `push` agree, but the machinery hides the order of
  events and a new setting costs a type and a registration.

## State

Everything the tool manages is expressed as flat entries: a key and a value,
for example `settings.fppMode = remote` or `network.eth0.address = 10.0.0.5`.
Lists (authorized keys, output processors, proxies) are one indexed entry per item, so
order, duplicates and an empty list all show; they are still written whole.

An entry is a fact about the controller, not only a stored value. Where the
stored value is not the whole truth the section adds entries for the rest:
whether an interface's stored config is the one in effect, whether the setup
gate is down.

A value is plain text. No secret is ever an entry's value: where the tool
manages one, the entry says only whether it is set (see Secrets). A failed
read is an error and stops the run; it is never taken as leave to overwrite.

Each section produces the wanted state from the config and the observed state
from the controller, in this same form.

- `diff` renders both to two files and runs the diff program on them.
- `push` compares once, prints that comparison, and applies exactly it. A
  section is handed its own changes; it does not read and decide again. The
  one exception is a `Rewriter` section (passwords). What it writes cannot be
  read back, so diff leaves it out, and push applies it on every run, even
  when nothing else changed.
- `diff` exits with the diff program's status; `push` stops if the program
  fails (status above 1).
- `diff --full` is the same reading with the keys the config leaves unset
  included.

## Sections

```go
type Section interface {
    Name() string
    Want() State                          // from the config
    Observe(ctx, *Target) (State, error)  // from the controller
    Apply(ctx, *Target, []Change) (After, error)
}

// After, in the tool's own order of severity:
// none < quick restart < restart < reboot.
```

The sections are a slice literal, in the order `push` runs them:

1. keys
2. settings
3. setup gate
4. audio
5. output processors
6. proxies
7. passwords
8. network (last)

There is no dependency graph; the order is this list. Around it, `push`:

1. logs in, reports a version mismatch, and refuses unless the status reads
   idle (an unreadable status is not idle) or `--force` is given;
2. observes, compares and prints;
3. applies sections 1 to 7;
4. takes the largest `After` and restarts fppd once, waiting until a new
   fppd answers (its uptime is lower than before, or it started later: the
   board's uptime less fppd's; a quick restart answers at once, while the old
   fppd still runs; if fppd was not answering before, any fppd that answers
   counts);
5. applies the network and confirms the result;
6. reboots, if anything needs one, and waits the same way. The reboot comes
   after the network because booting brings up whatever network config is
   stored: applied and confirmed first, the board comes back where the tool
   expects it.

A section probes for what it needs (the gate section for the consent API, the
network section for the gateway call). A feature the controller's FPP lacks
is reported and skipped; where the older FPP has another way (the gateway
inside the interface on 8.x) the section uses that. There is no version
table.

Each setting has one owner. The settings section does not write what another
section owns.

- **Keys.** Observes only the copy FPP stores
  (`GET /api/configfile/authorized_keys`; a 404 means no copy, so no keys).
  Keys put in the real files another way do not show. Entries are
  `keys.count` and `keys[0]`, `keys[1]`, ... Applies by uploading the whole
  file, which FPP installs over both root's and fpp's `authorized_keys`.
- **Setup gate.** Owns `LegalJurisdiction`, the four privacy settings, the
  consent record and `initialSetup-02`. It writes the privacy settings even
  when the value read back already matches (a default reads the same as a
  stored value), records consent with the final values, checks the reply
  body (an error comes back as 200), and reads the gate again afterwards.
  The consent date is made when writing and is not an entry. The config must
  give the jurisdiction and all four privacy settings (the schema requires
  them), so consent is never recorded for a value nobody chose.
- **Audio.** The volume, which has its own API call. `AudioOutput` is an
  ordinary setting.
- **Output processors.** Asks for a quick restart after any change. 8.4.2
  needs it, 10.4 does not, and nothing tells the two apart.
- **Proxies.** The hosts FPP proxies at `/proxy/<host>/`, written whole
  (`POST /api/proxies`; no proxies is `DELETE`, which 8.x lacks). Entries
  are `proxies.count` and `proxies[0]`, ... as the host and the quoted
  description. FPP proxies each address its own DHCP server leases, and
  saves those into the list whenever it writes it; they are left out of
  the observed list unless the config names the host, and written back as
  they were, so `DELETE` only runs when nothing is left. A change needs
  nothing afterwards: FPP reloads Apache itself.
- **Passwords.** Owns `password`, `passwordEnable`, `osPassword` and
  `osPasswordEnable`, written in the order FPP needs, and switches the HTTP
  client to the new password at that point. An empty password in the config
  is an error (FPP would turn it into `falcon`). Its only entries are the
  enable flags. It is a `Rewriter`: each push writes the passwords, and a
  write the API accepts is taken as done.
- **Network.** Saves every interface, DNS and gateway first, which changes
  nothing live. Then applies the interfaces that differ or are unapplied,
  the one the tool is talking through last. The order is a courtesy, not a
  guarantee: FPP's apply brings up every stored interface, whichever one is
  named, so the first apply can already move the management address and a
  later one then gets no answer. An apply that times out and one
  that answers 200 are treated alike: the test is that the controller then
  answers at each static address the config gives it, and at the management
  address. A DHCP interface is checked for a leased address (`ip` marks it
  `dynamic`); the old static address can linger after the apply answers.
  A gateway of `"dhcp"` is the empty gateway, shown as `dhcp` on both sides
  of the diff. It is cleared globally where FPP has that, and in every
  interface the config names that stores one: that is where 8.x keeps it,
  and 10.4 would otherwise take it back from there. Every named interface is
  then applied. DNS of `"dhcp"` is the empty server list.

## Settings

- The config names settings by FPP's own names (`HostDescription`, `fppMode`).
  There is no friendly-name layer.
- A CUE schema lists the managed settings with their types. It is closed: a
  name not in the schema is a config error. That list is also what
  `diff --full` shows.
- The tool reads the controller's setting definitions and takes from them
  whether the setting exists on this FPP (if not: skipped and reported), the
  allowed options, and whether a change needs a restart or reboot.
- Adding a setting is one line in the schema. Go code names a setting only
  when it needs special handling.
- The optional values in the schema are `T | null` (not the fields inside an
  interface). The loader reads a null
  as an absent field, so nothing past the `config` package knows about it.
  The `| null` makes CUE report a bad value twice over (once per
  alternative, under a header); the loader drops the header and the
  "is not null" line and keeps the real problem.
- `AudioOutput` differs by version (a card index on 8.x, an ALSA name on
  10.4). The config carries the value for that controller, checked against
  the controller's options.

## Secrets

FPP 10.4 does not return the UI password, the OS password or the WiFi PSK over
the API.

- **Passwords:** not compared and not shown. Push writes them on every run,
  which costs two API writes.
- **WiFi PSK:** the entry `network.<if>.psk` is `set` or `not set`, and is
  there only when the config gives a PSK (it then wants `set`). 10.4 reports
  `PSKSet`; where 8.x returns the PSK itself, only whether it is empty is
  used. Push writes the config's PSK when none is set, and whenever it saves
  that interface for another change. A config without a PSK leaves it out of
  the save, so FPP keeps the saved one (8.x needs the saved one sent back,
  so it is). A change to the PSK alone is not seen: rewriting it on every
  push would re-apply WiFi every time.

Secrets are never put in an entry, an error or a note.

## Transport

FPP's HTTP API, behind a small interface whose client handles the login
policy. Tests replay recorded responses at that seam. The tool does not use
SSH.

A link-local address without a zone gets one before the client sees it. If
only one of the operator's interfaces is up with a link-local address, that
interface is used. If several are, the tool tries TCP port 80 through each at
once and uses the one that answers. When none or more than one answers, it
stops and asks for `--at ADDRESS%IFACE`.

FPP's API is plain HTTP with Basic auth, so passwords and the PSK cross the
show network in the clear. FPP offers nothing else; the show network is
trusted for this.

## Packages

- `config`: the CUE schema, loading and Go types. Importable by other tools.
- internal packages: `fpp`, the API client; the sections; the diff and push
  loop.

## Left out on purpose

- A planner or dependency graph: the order is fixed and short.
- Rollback or transactions: FPP keeps backups.
- Diff rendering of our own: the external program does it.
- A Go struct field per setting, or friendly names mapped to FPP names.
- A local state file remembering the last push.
- Version comparisons, a plugin registry, concurrency.
