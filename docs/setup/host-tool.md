# Installing trustctl

`trustctl` is the command line client. It is one static binary without dependencies; it also carries
the firmware images, so it flashes the device without `avrdude`.

## Download

Download the archive for your system from the
[releases](https://github.com/schorsch3000/TrustIssue/releases). The `nightly` prerelease is the latest
commit on `master`. An archive holds `trustctl`, `trustctl-import` and the tools from `tools/user`
(Windows: also `trustctl-tray.exe`). `SHA256SUMS` lists the checksums, and every archive has a build
attestation:

```sh
gh attestation verify trustctl-linux-amd64.tar.gz --repo schorsch3000/TrustIssue
```

`trustctl` is released together with the firmware: the version of the built-in firmware is shown by

```console
$ trustctl version
trustctl 0.2.1, built-in firmware 0.2.1
```

## Build from source

```sh
cd trustctl
go build -o bin/trustctl ./cmd/trustctl
```

You need Go 1.25 or newer (the module fetches the toolchain it needs by itself). `CGO_ENABLED=0`
builds work for Linux (amd64, arm64, arm, 386), Windows (amd64, arm64), macOS (amd64, arm64), FreeBSD
and OpenBSD. NetBSD is not supported by the serial library. Only Linux amd64 is covered by the
end-to-end tests; the Windows build has been tried by hand.

## Tell it where the device is

`trustctl` needs a serial port. Either per command:

```sh
trustctl --device /dev/ttyUSB0 list
```

or once, in the configuration file `~/.config/trustctl/config` (`$XDG_CONFIG_HOME` is honoured;
`trustctl help-options` prints the exact path on your system):

```ini
# my device
device = /dev/ttyUSB0
idle_timeout = 10m
```

`TRUSTCTL_DEVICE` does the same in the environment. The serial ports of cheap Nanos change their
number after a replug; on Linux a udev rule or a `/dev/serial/by-id/...` path keeps it stable.

There is no implicit probing: opening a port resets whatever Arduino is on it. `--autoprobe` (or
`autoprobe = true` in the config file) tries every port until one answers; leave it off if anything else
is plugged in. `trustctl help-options` prints every option with its command line argument, environment
variable, configuration file syntax and default, see {doc}`../reference/trustctl`.

On Linux the user must be allowed to open the port (`dialout` group on Debian and Ubuntu, `uucp` on
Arch).

## Try it without a device

`--device DUMMY` (any case) uses a simulated device that follows the firmware's rules and accepts
every confirmation. It lives in the background daemon, so its keys stay from one command to the next:

```console
$ trustctl --device DUMMY add GitHub
Secret (Base32) or otpauth:// URI: ****************
Adding "GitHub" (6 digits, 30 s): check the name on the display.
Confirm on the device: hold the button for 1 to 2 seconds, then let go. A tap or a longer hold cancels.
stored as slot 0
$ trustctl --device DUMMY token github
Requesting the code for "GitHub".
Confirm on the device: hold the button for 1 to 2 seconds, then let go. A tap or a longer hold cancels.
982730
```

`trustctl daemon stop` gives a fresh, empty simulated device; nothing is ever stored on disk. For a
one-shot experiment `--simulate` is simpler (a fresh device inside every process). Neither works with
`flash` or `calibrate`.

(daemon)=
## The daemon

The first command starts a daemon (it re-executes itself, detached) that holds the serial port open
and caches the key names. Later commands talk to it over a Unix socket in `$XDG_RUNTIME_DIR` (mode
0600). It exits after an idle timeout (default 5 minutes, `idle_timeout` in the config file), when the
device disconnects, when the machine was suspended, or on `trustctl daemon stop`. `flash` and
`calibrate` stop it first. `--no-daemon` (or `TRUSTCTL_NO_DAEMON=1`, or `daemon = false`) talks to the
device directly. A running daemon holds one device: asking for another `--device` is an error until
`trustctl daemon stop`.

On Linux, macOS, FreeBSD and OpenBSD the port is opened without raising DTR again, so later commands do
not reset the device (and do not wake its display). On Windows and other systems every open resets it
until the daemon holds the port, so the first command after a pause takes 1 to 2 seconds longer.

## Tab completion

```sh
source <(trustctl autocomplete)             # bash and sh-compatible shells; put it into ~/.bashrc
source <(trustctl autocomplete-zsh)         # zsh
trustctl autocomplete-fish | source         # fish
```

It completes commands, options, key names (asked from the device, quick with the daemon), the
sub-commands of `tz`, `config`, `daemon` and `calibrate`, and file names for `flash` and `calibrate`.
The zsh and fish scripts follow the shells' documented conventions but have not been run by the
author.
