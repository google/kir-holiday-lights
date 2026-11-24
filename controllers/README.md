# Controller configs

One config per FPP controller, applied with `fppcfg` (built from
[tools/fppcfg](../tools/fppcfg): `cd tools/fppcfg && go build .`):

- `NNN-description.cfg`: one controller, where NNN is the last part of its
  wired address.
- `defaults.cue`: shared by every config here.
- `standby.cfg`: a spare on DHCP.

Run fppcfg from a machine on the show network (the gateway).

## Secrets

`secrets.cue` holds the SSH keys every controller gets and the WiFi password
the gift boxes need. It is gitignored; make it once:

    cd controllers
    cp secrets.cue.example secrets.cue     # then put the real values in

Never commit or share `secrets.cue`. Without it no config loads.

## Configure a controller

From the top of the repository:

    tools/fppcfg/fppcfg controllers/40-k16-water-tower.cfg diff     # shows changes, writes nothing
    tools/fppcfg/fppcfg controllers/40-k16-water-tower.cfg push     # shows them and applies them

- Without `--at`, fppcfg talks to the wired address in the config.
- A controller that is somewhere else (a fresh image on DHCP, the spare) needs
  `--at ADDRESS`.
- Moving the interface fppcfg is connected through to DHCP, over IPv4, loses
  the controller halfway: fppcfg warns and then does it, and its check after
  the apply fails. (A new static address is followed.) Use the board's IPv6
  link-local address instead. It does not change:

      ip neigh show 192.168.1.15                # the board's MAC address
      ping -6 -c2 ff02::1%IFACE                 # IFACE: the gateway's show-network interface
      ip -6 neigh show dev IFACE                # the fe80:: address with that MAC
      tools/fppcfg/fppcfg controllers/40-k16-water-tower.cfg push --at 'fe80::…%IFACE'

- `push` refuses while the controller is playing, unless given `--force`.
- A hostname change needs a reboot of the controller to take effect.
- `push` also installs the SSH keys in `defaults.cue`.
