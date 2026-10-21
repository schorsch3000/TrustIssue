# Everyday use

This page is the short version. Every command is described in {doc}`../reference/trustctl`.

## Add a key

`trustctl add` asks for the secret on the terminal and shows it as stars while you type or paste it. That
is the way to do it: the secret never appears on a command line, so it cannot end up in your shell history
or in the process list.

```console
$ trustctl add GitHub
Secret (Base32) or otpauth:// URI: ****************
Adding "GitHub" (6 digits, 30 s): check the name on the display.
Confirm on the device: hold the button for 1 to 2 seconds, then let go. A tap or a longer hold cancels.
stored as slot 0
```

Instead of the Base32 secret you can paste an `otpauth://` URI, as in the QR code a service shows you. It
carries its own name, digits and period, so the name can be left out:

```console
$ trustctl add
Secret (Base32) or otpauth:// URI: ****************************************
Using the name "Example:bob@example.com" from the URI.
```

:::{warning}
Do not put the secret on the command line, and do not `echo` it into a pipe by hand
(`echo SECRET | trustctl add ...`): the shell keeps the line in its history, and while it runs the secret is
visible in the process list. Reading from standard input is meant for **automation, where the secret is
never a parameter**, for example from a password manager:

```console
$ pass show totp/github | trustctl add GitHub
```
:::

On the device the display shows the name and the parameters. Hold the button and **let go while the bar is a
checkerboard** (that is the 1 to 2 second window; before it is too early, after it too late, both are a
*no*, see {ref}`reading-the-bar`):

```{image} /_static/display/add.gif
:alt: trustctl add GitHub: the confirm screen, the button held for 1.5 seconds, Done.
```

## Get a code

```console
$ trustctl token github
Requesting the code for "GitHub".
Confirm on the device: hold the button for 1 to 2 seconds, then let go. A tap or a longer hold cancels.
982730
```

The key can be given by name (any case), by slot number or by a unique beginning of a name. The
code is printed on standard output only, so `code=$(trustctl token github)` works. The device shows the key
name, its timezone offset and the **date and time the code is for**: check them against your own
clock. After the press it shows the code in large digits for five seconds.

```{image} /_static/display/token.gif
:alt: trustctl token GitHub: the confirm screen with the name, UTC+2 and the time; the bar while the button is held; Done and the code.
```

If the device's timezone differs from the host's (daylight saving, a trip), the first request first
asks you to confirm a timezone change on the device; see {ref}`cmd-tz`.

## Delete a key

```console
$ trustctl del Mastodon
```

```{image} /_static/display/del.gif
:alt: trustctl del Gitea: the confirm screen, the button held, Done.
```

## When you let go at the wrong time

Released before 1 second, a tap counts too: `Denied`.

```{image} /_static/display/result-denied.gif
:alt: A short press is denied.
```

Held past 2 seconds: `Too long`, at once, without waiting for you to let go.

```{image} /_static/display/result-too-long.gif
:alt: A hold past 2 seconds is denied at once.
```

No answer for 30 seconds: `Timeout`.

```{image} /_static/display/result-timeout.gif
:alt: No answer for 30 seconds is a timeout.
```

`trustctl` prints `trustctl: device: refused on the device (the button was not held for the right time)` or
`trustctl: device: nobody confirmed on the device in time` and exits with status 2. Just ask again.

## Wipe everything

`trustctl wipe` needs a long hold, 5 to 6 seconds, so you have time to reconsider:

```{image} /_static/display/wipe.gif
:alt: trustctl wipe: hold 5 to 6 seconds; the display is the progress bar while the keys are overwritten.
```

Timezone and settings survive a wipe. Without a computer, see {ref}`cmd-config` (`autowipe`).

## Keep a copy of the secrets

There is no export, ever. Keep the original secrets or QR codes in a safe place. Delete and add are the only
ways to change a key.

## Use it from a launcher

Linux and the BSDs have {doc}`../tools/trustctl-menu` (a dmenu/rofi style picker that types or copies the
code), Windows has {doc}`../tools/trustctl-tray`, macOS {doc}`../tools/trustctl-menubar`. To move an existing Google Authenticator onto the device
there is {doc}`../tools/trustctl-import`.
