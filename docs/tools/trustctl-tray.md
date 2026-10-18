# trustctl-tray (Windows)

`trustctl-tray.exe` is the Windows counterpart of {doc}`trustctl-menu`: a tray icon, an optional global
hotkey and a dmenu-like picker. It is in the Windows archive of every release.

Clicking the icon, or pressing the hotkey, lists the keys with a text field that filters while you type:
several words must all match, names that start with the first word come first. Up, Down and Enter or a
click choose, Esc (or clicking elsewhere) closes. The device then waits for the button (a balloon says
so), and the code is typed into the window that was active before, with an optional Enter after it, or
copied to the clipboard.

## Settings

The icon's menu has *Get a code*, *Settings...* and *Exit*. The first start opens the settings.

- the path of `trustctl.exe`, and an optional port such as `COM5` (passed as `--device`),
- *Sort the keys by name* (otherwise they come in the device's slot order),
- the hotkey: click the field and press the keys (it needs a modifier),
- type the code into the previous window (with an optional Enter and a delay in milliseconds) or copy it
  to the clipboard,
- start with Windows (`HKCU\...\Run`).

They are stored in `%APPDATA%\trustctl\tray.ini`. The code is never written anywhere or put on a command
line: `trustctl list` and `trustctl token` run without a window and their output is read from standard
output.

## Build

```bat
tools\user\trustctl-tray\build.cmd
```

uses the C# compiler that ships with Windows (.NET Framework 4, no SDK to install) and writes
`trustctl-tray.exe` next to the source. On Linux `tools/user/trustctl-tray/test.sh` compiles the source
with Mono and runs the tests of the filter, the argument quoting, the hotkey and the settings code.

On Windows `trustctl` resets the device on every port open until the daemon holds the port, so the first
list after a pause may take one to two seconds.
