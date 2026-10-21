# trustctl-menubar (macOS)

`trustctl-menubar.app` is the macOS counterpart of {doc}`trustctl-tray` and {doc}`trustctl-menu`: an icon
in the menu bar, an optional global hotkey and a dmenu-like picker. Its sources are in the macOS archives of
every release (`tools/user/trustctl-menubar/`); build it with `build.sh` (below).

Clicking the icon, or pressing the hotkey, lists the keys with a text field that filters while you type:
several words must all match, names that start with the first word come first. Up, Down and Return or a
click choose, Esc (or clicking elsewhere) closes. The device then waits for the button (a small notice at
the top right of the screen says so), and the code is typed into the app that was active before, with an
optional Return after it, or copied to the clipboard.

## Settings

A right click (or Ctrl-click) on the icon opens its menu: *Get a Code…*, *Settings…* and *Quit*. The first
start opens the settings.

- the path of `trustctl`; empty searches the `PATH`, `/opt/homebrew/bin`, `/usr/local/bin`, `~/bin`,
  `~/.local/bin` and `~/go/bin` (an app started from the Finder does not get the shell's `PATH`),
- an optional port such as `/dev/cu.usbserial-110` (passed as `--device`); `TRUSTCTL_DEVICE` from a shell
  profile does not reach an app, `device =` in `~/.config/trustctl/config` does,
- *Sort the keys by name* (otherwise they come in the device's slot order),
- the hotkey: click the field and press the keys (it needs Ctrl, Opt or Cmd; Delete clears it),
- type the code into the previous app (with an optional Return and a delay in milliseconds) or copy it to
  the clipboard,
- *Start at login* (a LaunchAgent, `~/Library/LaunchAgents/de.trustissue.trustctl-menubar.plist`).

They are stored in `~/Library/Application Support/trustctl/menubar.ini`. The code is never written anywhere
or put on a command line: `trustctl list` and `trustctl token` run without a window and their output is read
from standard output. A copied code is marked as concealed, so clipboard managers that follow
nspasteboard.org leave it out of their history.

Typing into another app needs the **Accessibility** permission (System Settings > Privacy & Security >
Accessibility); macOS asks for it the first time. The hotkey itself needs no permission. The build is signed
ad hoc, and macOS ties the permission to the signature: after a rebuild, remove the old entry and allow the
new one.

## Build

```sh
tools/user/trustctl-menubar/build.sh
```

uses the Swift compiler of the Command Line Tools (`xcode-select --install`, no Xcode needed) and writes
`trustctl-menubar.app` (arm64 and x86_64, macOS 12 or later) next to the sources; move it to
`/Applications` if you like (then switch *Start at login* off and on once, the LaunchAgent names the path).
`tools/user/trustctl-menubar/test.sh` runs the tests of the list parsing, the filter, the hotkey, the search
for `trustctl` and the settings code.
