# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is here

The KIR Holiday Light Show: xLights layout and sequences, one config per FPP
(Falcon Player) controller, and the Go tool that applies those configs.

- `xlights/`: the xLights show directory (`xlights_rgbeffects.xml` is the
  layout, `xlights_networks.xml` the controllers, `*.xsq` the sequences).
  `docs/xlights-format.md` has the file formats, the edits xLights accepts
  and its automation interface. Make the edits it covers by hand. For one it
  doesn't cover yet, predict the XML change, make it through xLights, compare,
  and record what differed there, so more edits can be made by hand over time.
  `*.fseq` (rendered output), `Media/` and `local/` (backups) are gitignored;
  after changing a sequence, re-render its `.fseq` (see "Rendering without the
  GUI" in that doc). The Nix package used here installs the binary as
  `xLights`; there is no lowercase `xlights` on the PATH.
- `controllers/`: `NNN-description.cfg` per controller (NNN is the last octet
  of its wired address), `defaults.cue` shared by all, `standby.cfg` for a
  spare on DHCP. `secrets.cue` is gitignored (copy `secrets.cue.example`);
  without it no config loads. `controllers/README.md` is the operator guide.
- `tools/fppcfg`: brings one controller to the state its config describes.

## fppcfg

Run from `tools/fppcfg`:

    go build .                                   # binary ./fppcfg (gitignored)
    go test ./...                                # engine tests take ~10 s
    go test ./internal/section -run TestNetworkWant
    go vet ./... && gofmt -l .

    ./fppcfg ../../controllers/40-k16-water-tower.cfg diff [--full] [--at ADDR]
    ./fppcfg ../../controllers/40-k16-water-tower.cfg push [--force] [--at ADDR]

`diff` only reads. `push` writes to a live controller, so treat it as a
change to the show. The diff program is `$FPPCFG_DIFF`, then `$DIFF`, then
`diff -u`.

Reach a controller by its IPv6 link-local address whenever its IPv4 address
is unknown or about to change (a fresh image, DHCP, a reboot on the bench).
It stays the same whatever the board's network config:

    ping -6 -c2 ff02::1%IFACE              # IFACE: the interface on the show network
    ip -6 neigh show dev IFACE             # the fe80:: address with the board's MAC
    ./fppcfg ../../controllers/….cfg diff --at 'fe80::…%IFACE'

fppcfg doesn't manage the pixel-string outputs (`co-bbbStrings.json`,
`co-pixelStrings.json`). They come from an xLights upload or are set in FPP
by hand.

`tools/fppcfg/docs/requirements.md` (what it must do), `design.md` (how it is
built) and `fpp-api.md` (how FPP's API actually behaves) are the source of
truth. Read design.md before changing the engine or a section. In short:

- Everything managed is flat `key = value` state. Each section produces the
  wanted state from the config and the observed state from the controller.
  `diff` renders both through the diff program; `push` compares once and
  applies exactly that comparison, so diff and push cannot drift.
- Sections (`internal/section`) are a fixed slice run in order: keys,
  settings, setup gate, audio, output processors, proxies, passwords, network
  (last).
  There is no dependency graph. The engine (`internal/engine`) restarts fppd
  once after sections 1–7, applies the network, then reboots once if needed.
- Each FPP setting has exactly one owning section.
- FPP settings are data: the CUE schema `config/schema.cue` (embedded) lists
  the managed settings by FPP's own names and is closed. Adding a setting is
  one line there; Go code names a setting only when it needs special handling.
  Optional schema fields are `T | null`, and the loader turns null into absent.
- Secrets (passwords, WiFi PSK) never appear as an entry's value, in errors or
  in notes. Passwords are a `Rewriter` section, written on every push.
- No version table: sections probe for the API they need and skip with a
  report when the controller's FPP lacks it.
- Tests replay controller responses recorded from real FPP 10.4 via
  `internal/fake` (`internal/fake/testdata/fpp<version>/`, e.g. `fpp10.4/`,
  one file per API path, `/` → `_`).
- `config` is importable by other tools; everything else is `internal/`.

## Conventions

- The repository is public. Keep private hostnames and domains, keys, PSKs,
  passwords and real hardware identifiers (MACs, serial-derived UUIDs,
  link-local addresses) out of code, docs and test data. Real values go in
  `controllers/secrets.cue`. The show's location is public.
- Comments and docs state current behaviour only: no notes about old versions
  or deleted files.
- Every code file in `tools/` starts with the Apache 2.0 header
  (`// Copyright 2026 Google LLC` …), followed by a blank line so it stays
  out of Go package doc comments.
