# fppcfg

Brings one FPP (Falcon Player) controller to the state described by one config
file, and shows the difference first.

    fppcfg ./station.cfg diff
    fppcfg ./station.cfg push

A config is a CUE file; every `.cue` file beside it (shared defaults, the
gitignored `secrets.cue`) is loaded with it. See [docs/example](docs/example).
[docs/requirements.md](docs/requirements.md) says what the tool must do and
[docs/design.md](docs/design.md) how it is built.

    go build .
