# How it works

## The device and the host

The device is the vault. The computer (the *host*) talks to it over the USB serial port, normally
through `trustctl`, and can ask for these things:

- add a key (a name, a secret and the parameters of the code),
- delete a key,
- list the key names (names are not secret),
- request the current code of a key,
- read and set the timezone offset of the device,
- read and change the device's settings, and wipe all keys.

There is no command that returns a stored secret. Nothing can read it back, not even the owner;
that is why you must keep the original secrets or QR codes as an offline backup.

## Every sensitive step needs the button

Adding, deleting, wiping, changing the timezone, changing settings and every code request show what
they are about on the display and wait for you. You confirm by **holding the button for a set time
and then letting go**:

| What                                     | Hold the button for                              |
| ---------------------------------------- | ------------------------------------------------ |
| add, delete, code, timezone, settings    | **1 to 2 seconds**                               |
| wipe all keys                            | **5 to 6 seconds**                               |
| wipe without a host (autonomous wipe)    | **10 to 11 seconds**, then a gesture, see below  |

Letting go too early or too late is a *no*: the request is denied and the host gets `ERR DENIED`
(`trustctl` exits with status 2). Hold past the end of the window and the device denies at once
without waiting for you to let go ("Too long"). Nothing happens if nobody answers for 30 seconds
("Timeout").

### Reading the bar

While the button is held, the whole display is a progress bar. Its full width is the **end** of the
accept window. The part before the window is solid; from the start of the window on, the bar is a
checkerboard. **Let go while the bar is a checkerboard.**

```{image} /_static/display/token.gif
:alt: A token request: the bar fills from left to right, solid first, then as a checkerboard from the middle on, and the button is released while the bar is a checkerboard.
:align: center
```

The text stays readable through the bar (the bar inverts it as it passes), so you can check the key
name and the time while you hold.

### A press that was already going does not count

If the button is down when a request arrives, that press is ignored: let go and press again. Malware
that sends a request while you happen to hold the button gets nothing.

## What the confirm screen shows

The first row says what is being done; the next rows show what matters for it:

- **Code:** the key name (two lines for long names), the timezone offset of the device in the corner
  (`UTC+2`) and the **local date and time** with a four digit year. The device has no clock: the host
  sends the time with each request, and the device shows the result so that you can compare it with
  your own clock. A code for another moment would show another time.
- **Add:** the name and the digits and period of the new key.
- **Delete:** the name.
- **Wipe:** the number of keys.
- **Settings:** all five settings, the ones this request changes inverted. The new rotation and
  inversion are already shown while you decide, and revert if you deny.

{doc}`display/screens` shows each of them.

## Results

After you decide, the display shows a big word for a moment: `Done`, `Denied`, `Too long` or
`Timeout`. After an accepted code request it shows `Done` and the code itself in large digits for
five seconds (a press ends it early), so you can read it off the device. The host has received the
code anyway; this reveals nothing new.

## Idle, logo and sleep

The idle screen shows the name, the number of keys and, if something is not finished, warnings, see
{ref}`idle-screen`. After 10 seconds without activity the display shows the idle logo (if enabled)
at full contrast for 30 seconds, then dimmed for 2 minutes, and then turns off. With no logo it turns
off after the 10 seconds. A button press wakes it. Commands that only read (`list`, `hello`, `tz get`,
`config get`) neither wake the display nor reset the timer, so a program polling the device cannot
keep it lit.

## Time and timezone

The device cannot handle daylight saving. It stores a fixed offset in 15 minute steps and `trustctl`
checks it before every code request, compares it with the host's current offset and, if they differ
(a clock change, a trip), sets it with an extra confirmation: `trustctl tz sync`. The offset is never
part of the same request as the time, so malware cannot ask for a time two hours ahead and pass it off
as correct by lying about the offset.

## The daemon

The first `trustctl` command starts a small background process that keeps the serial port open and
remembers the list of names. Later commands talk to it. This avoids the 1 to 2 seconds a USB serial
port needs to settle, and the display wakes less often. See {ref}`daemon`.
