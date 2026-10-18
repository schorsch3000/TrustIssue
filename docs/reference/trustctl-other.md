# trustctl: other commands

The commands that do not need the button, or do not use the device at all. The ones that do are in
{doc}`trustctl`.

## list

```text
trustctl list
```

Shows the keys: the slot number and the name. Names are not secret, so no confirmation is needed, and
`list` neither wakes the display nor resets its idle timer: a program that polls it cannot keep the device lit.

```console
$ trustctl list
 0  GitHub
 1  Mastodon
 2  Fastmail
```

The slot number is the key's id; it is stable until you delete the key. With the daemon the list is cached
and refreshed after every change.

## hello

```text
trustctl hello
```

Prints what the device says about itself: the firmware version, the protocol version and the build
capabilities and warnings:

```console
$ trustctl hello
TrustIssue 0.2.0 proto 1 caps=autowipe nopin unlocked
```

`caps=autowipe` is a build option (the autonomous wipe is built in), `nopin` means no update PIN is set
and `unlocked` that the lock bits and fuses are not those of a finished device. For the last two
`hello` (and `add`) also print a warning in words on standard error. A finished device says only
`TrustIssue 0.2.0 proto 1 caps=autowipe`. The same words are sent unprompted when the device starts.

## daemon

```text
trustctl daemon [status|stop]
```

Controls the background process that holds the serial port open, see {ref}`daemon`.

```console
$ trustctl daemon status
OK device=/dev/ttyUSB0 clients=1 idle=5m0s
$ trustctl daemon stop
OK
```

It starts by itself with the first command. `stop` also resets a `DUMMY` device to empty.

## version

```text
trustctl version
```

Prints the version of `trustctl` and of the firmware built into it. They are released together; a
`trustctl` binary always flashes exactly the firmware it was built with.

## font

```text
trustctl font list
trustctl font extract FILE.bin [--name NAME | --from FILE.hex] [--profile PATH]
```

`font list` shows the built-in fonts (the one that is the firmware's default is marked). `font extract`
writes a font as a 772 byte file to work on: the profile's current font, or with `--name` a built-in one, or with
`--from FILE.hex` the one in a firmware file. Without a profile font it is the built-in default. See
{doc}`../setup/calibration` and {doc}`../tools/misc` for turning it into a picture and back.

## logo

```text
trustctl logo extract FILE.png [--from FILE.hex] [--profile PATH]
```

Writes the current logo (the profile's, or the one in a firmware file) as a 128x32 grayscale PNG
(white = lit, already inverted if the profile says so) to work on. It is an error if there is no logo.

## help and help-options

```text
trustctl help
trustctl help-options
```

`help` prints the usage. `help-options` prints every option with its command line argument, environment
variable, configuration file line and default, with the real paths of the config file and the profile
on your system.

## autocomplete

```text
trustctl autocomplete          # bash and sh (also autocomplete-bash, autocomplete-sh)
trustctl autocomplete-zsh
trustctl autocomplete-fish
```

Print the completion code for the shell: see {doc}`../setup/host-tool`.

## The profile

`~/.config/trustctl/device.json` holds what `calibrate` remembers and `flash` patches in:

| Key              | Meaning                                                      | Default                  |
| ---------------- | ------------------------------------------------------------ | ------------------------ |
| `contrast_full`  | contrast of the display in use, 0 to 255                     | `0x20` (32)              |
| `contrast_dim`   | contrast of the dimmed logo                                  | `0x00`                   |
| `logo`           | path of the logo PNG                                         | none                     |
| `logo_invert`    | whether the logo is inverted                                 | `false`                  |
| `font`           | path of a font file, or `builtin:NAME`                       | the built-in Victoria font |
