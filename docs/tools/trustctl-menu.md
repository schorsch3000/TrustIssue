# trustctl-menu (Linux and BSD)

`tools/user/trustctl-menu` is a small POSIX shell script that lists the keys in a dmenu-like launcher,
asks the device for the code of the one you pick (you hold the button) and hands the code over: it prints
it, types it into the focused window or puts it into the clipboard. Bind it to a key in your window
manager.

```sh
trustctl-menu [chooser=PROGRAM] [target=stdout|type|copy] [delay=MS] [enter] [sort] \
              [clipboard=clipboard|primary|secondary] [-- CHOOSER-ARGUMENTS...]
```

## Parameters

`chooser=PROGRAM`
: A name from the `PATH` or a full path (`chooser=/bin/rofi`). Without it the first of `rofi`, `dmenu`,
  `wofi`, `bemenu`, `fuzzel` that is installed is used. rofi gets `-dmenu`, wofi and fuzzel `--dmenu`.

`sort` (or `sort=1`; `sort=0` turns it off)
: List the keys sorted by name, ignoring case. The default is the device's slot order.

`target=stdout` (the default)
: Print the code.

`target=type`
: Type the code into the focused window with `xdotool` (`wtype` on Wayland). `delay=MS` waits that many
  milliseconds before typing, to let go of the hotkey; `enter` (or `enter=1`) presses Return after the code.

`target=copy`
: Put the code into a clipboard with `xclip`, `xsel` or `wl-copy`. `clipboard=NAME` picks `clipboard` (the
  default), `primary` or `secondary`.

`--`
: Everything after it is given to the chooser: `trustctl-menu chooser=rofi -- -p token -theme-str
  'window {width: 20%;}'`.

## Examples

```sh
trustctl-menu                                        # first launcher found, prints the code
trustctl-menu chooser=rofi target=copy               # into the clipboard
trustctl-menu chooser=/bin/rofi target=copy clipboard=primary
trustctl-menu target=type delay=300 enter            # type it into the focused window
trustctl-menu sort                                   # sorted by name
```

## How it works

It needs `trustctl` in the `PATH`, and that must know the device (`device =` in its config file, or
`TRUSTCTL_DEVICE`; see {doc}`../setup/host-tool`). It takes the names from `trustctl list` (the slot numbers are
cut off) and runs `trustctl token NAME`.

If `notify-send` exists, a notification says that the device waits for the button. It is closed as soon as
`trustctl token` returns (accepted, refused or timed out), through `gdbus` or `busctl`; an old
`notify-send` without `-p` lets it expire after 30 seconds, when the device gives up anyway.

The code never appears on a command line: it goes to `xdotool`, `wtype`, `xclip`, `xsel` or `wl-copy`
through standard input.

Exit status: 0 done, 1 nothing chosen, 2 an error (also shown as a notification).

While you hold the button the display looks as in {doc}`../reference/trustctl` (`token`).
