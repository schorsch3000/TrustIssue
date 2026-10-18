# trustctl: commands that use the device

```text
trustctl [options] <command> [arguments]
```

This page covers the commands that talk to the device and, for most of them, wait for the button.
The others (`list`, `hello`, `daemon`, `version`, `font`, `logo`, `help-options`, `autocomplete`) are in
{doc}`trustctl-other`. `trustctl help` prints a short usage and `trustctl help-options` every option
with its environment variable and configuration file syntax.

## Global options

They may stand before or after the command (`trustctl --device /dev/ttyUSB0 list` and `trustctl list
--device /dev/ttyUSB0` are the same). A value comes from the first of these that is set: the command
line, the environment, the configuration file, the default.

`--device PATH`
: The serial port of the device, such as `/dev/ttyUSB0` or `COM3`. `DUMMY` (any case) is a simulated
  device that needs no hardware (see {doc}`../setup/host-tool`).
  Environment: `TRUSTCTL_DEVICE`. Config file: `device = PATH`. Default: none (a device is required,
  or `--autoprobe`).

`--autoprobe`
: Open every serial port that could be the device until one answers. Opening a port resets whatever
  Arduino is on it, so this is opt-in. Config file: `autoprobe = true`. Default: off.

`--simulate`
: Use a simulated device in memory that confirms everything; a fresh one for every command.
  Environment: `TRUSTCTL_SIMULATE=1`.

`--no-daemon`
: Talk to the device directly instead of through the background daemon. Environment:
  `TRUSTCTL_NO_DAEMON=1`. Config file: `daemon = false`.

The configuration file is `~/.config/trustctl/config` (`$XDG_CONFIG_HOME` is honoured): one `key =
value` per line, `#` starts a comment. Besides `device`, `autoprobe` and `daemon` it knows
`idle_timeout = 5m`, how long the daemon waits without a command before it exits (a Go duration such as
`90s`, `10m`, `1h`; there is no command line option for it).

Other environment variables that are read: `XDG_CONFIG_HOME` (where the config file and the profile
live), `XDG_RUNTIME_DIR` (the daemon's socket and log; else a private directory under the temporary
directory) and `HOME`.

**Confirming on the device:** hold the button and let go **while the bar is a checkerboard**; too early and
too late are both denied ({ref}`reading-the-bar`).

## Names and slots

`NAME` in `token` and `del` is a key name in any case, its slot number, or a unique beginning of a name.
A name beats a slot number (a key called `2` is found before slot 2). `trustctl list` shows the slots.

## Exit status

| Status | Meaning                                                         |
| ------ | --------------------------------------------------------------- |
| 0      | success                                                         |
| 1      | failure (no such key, name rejected, device error, ...)         |
| 2      | denied or timed out on the device (the button was not held right, or nobody answered) |
| 64     | usage error (the usage text is printed)                         |
| 69     | no device found                                                 |

Messages go to standard error; only data (`list`, the code of `token`, `config get`, ...) goes to
standard output, so `code=$(trustctl token github)` is safe.

---

## add

```text
trustctl add [NAME] [--digits N] [--period S]
```

Stores a key. `add` asks for the secret on the terminal and shows it as a masked star field while you type or
paste it. That is the way to use it by hand: the secret is **never a command line argument**, so it cannot end
up in your shell history or in the process list. The input is either a Base32 secret or an `otpauth://totp/...`
URI (as in the QR code a service shows you), which also brings the name, digits and period.

:::{warning}
Standard input is meant for **automation, where the secret is never a parameter**: when `add` reads from a pipe
it takes one line from it, for example from a password manager (`pass show totp/github | trustctl add GitHub`).
Do not `echo` a secret into it by hand (`echo SECRET | trustctl add ...`): the shell keeps the line in its
history, and the secret is visible in the process list while it runs.
:::

`NAME`
: The name shown on the device: 1 to 27 printable ASCII characters (space to `~`), no leading or trailing
  space, no two spaces in a row. Names are compared ignoring case (`github` is a duplicate of `GitHub`),
  but the case you give is kept for display. Without a `NAME`, a URI supplies one (`Issuer:account`, or
  just the label when it already starts with the issuer). A given `NAME` wins over the URI. A name that is
  too long or contains other characters is an error, never cut off or changed silently; only characters
  outside printable ASCII in a *URI label* (accents, emoji) are replaced by `-`, and `trustctl` says so.

`--digits N`
: Digits of the code, 6 to 8 (default 6). Only for a plain Base32 secret; a URI brings its own. The option
  comes after the `NAME`.

`--period S`
: Seconds a code is valid, 30 or 60 (default 30). Only for a plain Base32 secret.

The secret may have up to 20 bytes (SHA-1 only; a longer secret, SHA-256 or SHA-512, and HOTP are
rejected with a reason). The device stores 20 keys: a 21st gets `FULL`.

```console
$ trustctl add GitHub
Secret (Base32) or otpauth:// URI: ****************
Adding "GitHub" (6 digits, 30 s): check the name on the display.
Confirm on the device: hold the button for 1 to 2 seconds, then let go. A tap or a longer hold cancels.
stored as slot 0
```

On a device that is not finished (no lock bits, no update PIN) `add` prints the warnings first, before
real keys go in, see {doc}`../setup/first-use`.

**On the device:** hold 1 to 2 seconds.

```{image} /_static/display/add.gif
:alt: The confirm screen of add: ADD KEY, the name, the digits and the period; the button held; Done.
```

Errors found by `trustctl` before anything is sent, with the same rules as the device
(the device checks them itself too, a violation is `ERR BADARG`):

```console
$ trustctl add "Bad  name"
Secret (Base32) or otpauth:// URI: ****************
trustctl: the name "Bad  name" contains two spaces in a row
$ trustctl add example:BOB@example.com
Secret (Base32) or otpauth:// URI: ****************
trustctl: device: a key with this name already exists (names are compared ignoring case)
```

There is no `update`: to change a key, `del` it and `add` it again. That keeps malware from silently
overwriting one.

## token

```text
trustctl token NAME
```

Asks the device for the current code of a key and prints it. It first runs `tz sync` (see below), then sends
the host's current time, and the device waits for the button. The device shows what the code is for:
the key name, its timezone offset and the local **date and time**; compare them with your clock.

```console
$ trustctl token github
Requesting the code for "GitHub".
Confirm on the device: hold the button for 1 to 2 seconds, then let go. A tap or a longer hold cancels.
982730
```

**On the device:** hold 1 to 2 seconds. The result screen shows `Done` and the code in large digits for
5 seconds (a press ends it early).

```{image} /_static/display/token.gif
:alt: The confirm screen of token with the time, the bar while the button is held, and Done with the code.
```

The time keeps running while you hold: the device shows the host's timestamp plus the time that has
passed since the request arrived, and updates the minute live.

(cmd-del)=
## del

```text
trustctl del NAME
```

Deletes a key (the slot, its name and the secret are zeroed). **On the device:** hold 1 to 2 seconds.

```{image} /_static/display/del.gif
:alt: The confirm screen of del: DELETE KEY and the name; the button held; Done.
```

(cmd-tz)=
## tz

```text
trustctl tz [get]
trustctl tz sync
trustctl tz set +H[:MM]
```

The device has no clock and no daylight saving rule: it stores a **fixed offset** from UTC in quarter
hours (UTC-12 to UTC+14, including 45 minute zones) and shows it on every code request.

`tz get` (the default)
: Shows the device's offset and the host's.

  ```console
  $ trustctl tz get
  device: UTC+0
  host:   UTC+2
  ```

`tz sync`
: Sets the device to the host's current offset if it differs. This needs a confirmation. `token` does it
  automatically before each request.

  ```console
  $ trustctl tz sync
  The device is set to UTC+0 but this host is at UTC+2; setting the device's offset.
  Confirm on the device: hold the button for 1 to 2 seconds, then let go. A tap or a longer hold cancels.
  ```

`tz set +H[:MM]`
: Sets an offset explicitly (`+2`, `-5`, `+5:30`, `+5:45`). Anything outside UTC-12 to UTC+14 is rejected
  (`trustctl: the offset must be between UTC-12 and UTC+14`).

**On the device:** hold 1 to 2 seconds. This is the one place where you must know the correct offset:
the offset is never sent together with the time of a code request, so malware cannot make a wrong time
look right.

```{image} /_static/display/tz-set.gif
:alt: The confirm screen of tz set: SET TIMEZONE and UTC+2; the button held; Done.
```

(cmd-config)=
## config

```text
trustctl config [get]
trustctl config set FLAG=on|off [FLAG=on|off ...]
```

Reads and changes the settings that live **on the device** (in its EEPROM; they survive firmware
updates).

```console
$ trustctl config get
rotate        off
invert        off
logo          off
autowipe      off
show-version  off
```

| Flag           | Meaning                                                                                              |
| -------------- | ---------------------------------------------------------------------------------------------------- |
| `rotate`       | turn the display by 180 degrees (a hardware command, no cost)                                        |
| `invert`       | swap lit and dark pixels                                                                             |
| `logo`         | show the idle logo, and for 5 s after power-up (needs a logo in the firmware: {doc}`../setup/calibration`) |
| `autowipe`     | allow wiping all keys with the button alone, see below (only effective if built into the firmware: `caps=autowipe`) |
| `show-version` | show the firmware version on the idle screen                                                         |

`config set` needs a confirmation like any change, and the device shows all five settings with the ones
this request changes **inverted**. Rotation and inversion are applied at once, as a preview, and reverted
if you deny. **On the device:** hold 1 to 2 seconds.

```{image} /_static/display/config-rotate.gif
:alt: config set rotate=on: the confirm screen is already rotated, the button is held, Done, and the idle screen stays rotated.
```

![Settings screen with changed flags inverted](/_static/display/config-flags.png)

`trustctl config set show-version=on rotate=off` on a device where rotate and logo were on: the changed
flags are inverted.

`invert` works the same way:

```{image} /_static/display/config-invert.gif
:alt: config set invert=on: the confirm screen is inverted.
```

### The autonomous wipe

With `autowipe=on`, the keys can be destroyed without a computer, for example when the device must be
gotten rid of and only a power bank is at hand. At the idle screen **hold the button**: after 2 seconds a
bar appears, and you let go **between 10 and 11 seconds**. Then `WAIT` and a four second countdown (a
press cancels), then the same gesture as the bootloader: **tap, tap, press a third time and hold for 2
seconds**. A mistake starts the gesture over; after 20 seconds the whole thing is cancelled. It only
arms when there are keys, and during it every serial command gets `ERR BUSY`. It can only destroy, so
it is off by default. The wipe keeps the timezone and the settings.

```{image} /_static/display/autowipe.gif
:alt: The autonomous wipe: the hold bar, the countdown, tap tap hold, the wipe.
```

## wipe

```text
trustctl wipe
```

Erases **all** keys. The window is 5 to 6 seconds of holding, long enough to notice and reconsider.
On a device with no keys nothing is asked. The secrets are overwritten interleaved across all slots (byte 0
of every key, then byte 1, ...), so a wipe that is interrupted by pulling the power leaves every key equally damaged
instead of some intact; then names, checksums and the slot flags are cleared. The whole display is the
progress bar of the wipe.

```console
$ trustctl wipe
Erasing ALL keys on the device.
Confirm on the device: hold the button for 5 to 6 seconds, then let go. Any other time cancels.
```

```{image} /_static/display/wipe.gif
:alt: trustctl wipe: the confirm screen, the bar over 5 to 6 seconds, then the wipe progress bar.
```

Releasing before 5 seconds (for example a 1.5 second hold) is a plain denial:

```{image} /_static/display/wipe-denied.gif
:alt: A 1.5 second hold on the wipe screen is denied.
```

## flash

```text
trustctl flash [core|calibration] [OPTIONS]
trustctl flash FILE.hex [OPTIONS]
trustctl flash pin --set-pin | --clear-pin
```

Programs the firmware through the bootloader. This needs the button gesture and, if one is set, the
update PIN: the full procedure with pictures is {doc}`../setup/flashing`. `core` (the default) is the
firmware built into `trustctl`, `calibration` the calibration firmware ({doc}`../setup/calibration`),
`FILE.hex` a firmware file of your own (a name wins over a file; use `./core` for a file called core).
The device is reset once; `trustctl` then retries syncing for up to 90 seconds while you do the
gesture.

`--no-patch`
: Flash the firmware as it is, without the font, logo and contrast of the profile. (The calibration
  firmware is never patched.)

`--profile PATH`
: The profile file to patch in (default `~/.config/trustctl/device.json`).

`--font NAME|FILE.bin`
: Patch this font in instead of the profile's (a built-in name from `trustctl font list` or a font file). After a
  successful flash it is stored in the profile.

`--pin`
: Ask for the current update PIN before the gesture (the device waits only 8 s for it).

`--set-pin`
: Write a new update PIN (4 to 15 characters, asked for twice) with this flash.

`--clear-pin`
: Remove the update PIN with this flash (the current one is asked for).

`--wait DURATION`
: How long to wait for the bootloader to open (default `90s`).

`--countdown N`
: Seconds between "hold the button" and the reset (default `4`).

`flash pin --set-pin|--clear-pin` writes only the PIN page and leaves the firmware as it is. `flash` stops
the daemon first, refuses anything that touches the bootloader's section or the PIN page, and needs `--device`.

```{image} /_static/display/flash-gate.gif
:alt: The bootloader gate shown while trustctl flash waits.
```

## calibrate

```text
trustctl calibrate contrast full|dim VALUE
trustctl calibrate logo FILE.png [--invert]
trustctl calibrate font NAME|FILE.bin
trustctl calibrate rotate on|off
trustctl calibrate invert on|off
trustctl calibrate show
trustctl calibrate forget logo|font|contrast
```

Tries out the look of the display live on the **calibration firmware** (`trustctl flash calibration`)
and remembers the choice in the profile, which `flash` patches into the core firmware. They need no
button. `--profile PATH` after the sub-command selects another profile file. The pictures of each one
are on {doc}`../setup/calibration`.

`contrast full|dim VALUE`
: Contrast of the display in use and of the dimmed logo, 0 to 255 or `0x..` (defaults `0x20` and
  `0x00`).

`logo FILE.png [--invert]`
: Shows the PNG (at most 128x32; one pixel on or off, no dithering, see {doc}`../setup/calibration`) and remembers it.

`font NAME|FILE.bin`
: Shows a test text with a built-in font or a 772 byte font file and remembers it (`builtin:NAME` in the
  profile).

`rotate on|off`, `invert on|off`
: Try the 180 degree rotation or the inversion. These are not remembered in the profile because they
  are device settings: use `config set rotate=on` on the core firmware.

`show`
: What `flash` will patch into the firmware.

`forget logo|font|contrast`
: Drop a remembered setting.
