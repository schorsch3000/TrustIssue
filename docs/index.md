# Trust Issue

A hardware TOTP vault for people who don't trust their computers.

The secrets live on a small device (an Arduino Nano with a 128x32 OLED and one button) and never leave
it. The computer can add keys, list their names and ask for a code, but every one of those needs
somebody to hold the button on the device while its display shows what is being done. Malware on the
laptop can neither export the secrets nor silently use them, and it cannot reflash the device either:
the bootloader only opens after a gesture on the button.

```{image} /_static/device/device.jpg
:alt: The finished device: a black 3D printed case with a red button and the display, connected by USB.
:align: center
:width: 70%
```

```{image} /_static/display/token.gif
:alt: The display while a code is requested with "trustctl token GitHub": the name, the date and time, a bar while the button is held, then the code.
:align: center
```

**The one thing to know:** to confirm anything on the device, hold the button and let go while the bar on the display is a checkerboard (see {ref}`reading-the-bar`). Too early and too late are both a no.

*Every picture of the display in this documentation is made by running the real firmware on a
computer, see {doc}`development/display-images`. The animations show the button too, with the time it
has been held.*

The project is on [GitHub](https://github.com/schorsch3000/TrustIssue): source, releases, issues.

## Who it is for

Trust Issue is for technically comfortable people who want a DIY, physically mediated TOTP vault: you
solder a few wires, flash a microcontroller and use a command line. It is **not** a drop-in replacement for a
certified security key (see {ref}`why-not-a-yubikey`), and it does not make TOTP phishing-resistant.

## What you get

- **The hardware device and firmware:** an Arduino Nano with a small OLED and one button, a button-gated
  bootloader and the firmware, with a 3D printable case.
- **`trustctl`, the command line interface:** add, list and delete keys, get codes, flash and calibrate the
  device, in one static binary for every common platform.
- **Desktop integrations and import tools:** a launcher menu for Linux and the BSDs, a Windows tray tool and an
  importer for Google Authenticator exports.
- **Documentation for developers and people building compatible tools:** the serial protocol, the storage
  layout, the security model, build and test instructions, and a host simulation of the firmware.

## Where to start


:::{list-table}
:header-rows: 0
:widths: 18 82

* - **Build one**
  - {doc}`hardware/wiring` (parts and wires), {doc}`hardware/case` (3D printed case), then {doc}`setup/onboarding` (bootloader and firmware, once, over ISP).
* - **Set it up**
  - {doc}`setup/host-tool` (install `trustctl`), {doc}`setup/flashing` (updates over USB), {doc}`setup/calibration` (font, logo, contrast), {doc}`setup/first-use`.
* - **Use it**
  - {doc}`concepts` (how the button and the display work), {doc}`usage/daily`, {doc}`reference/trustctl`.
* - **Look things up**
  - {doc}`display/screens` (every screen the device can show), {doc}`reference/protocol`, {doc}`troubleshooting`.
* - **Understand the security**
  - {doc}`security/model`, {doc}`security/physical-access`.
:::


(why-not-a-yubikey)=
## Why not a YubiKey?

If a service supports FIDO2/WebAuthn, use a security key there: it is phishing-resistant, which a TOTP
code never is, and it is better engineered than this project. Trust Issue is for the many services
that only offer TOTP. It differs from a YubiKey's OATH feature in a few ways:

- **You see what you approve.** The display shows the key name and the local date and time of the code,
  and a code needs a deliberate 1 to 2 second hold. A YubiKey has no display: the host decides which
  credential and which time to ask for, and a touch is a context-free tap that malware can time.
- **Open and auditable.** The firmware, the bootloader and the host tool are all here, under the GPL.
  Updates need a physical gesture and can be protected with a PIN.
- **Cheap and replaceable.** An Arduino Nano and a small display cost a fraction of a security key.
- **What you give up.** An ATmega328P is not a certified secure element and does not resist attacks on
  the chip (see {doc}`security/physical-access`). It only does TOTP, holds 20 keys, and is a
  do-it-yourself build.

Both can be used together; they solve different parts of the problem.

```{toctree}
:hidden:
:caption: Build and set up

hardware/wiring
hardware/case
setup/onboarding
setup/host-tool
setup/flashing
setup/calibration
setup/first-use
```

```{toctree}
:hidden:
:caption: Use

concepts
usage/daily
display/screens
```

```{toctree}
:hidden:
:caption: Reference

reference/trustctl
reference/trustctl-other
reference/protocol
reference/storage
```

```{toctree}
:hidden:
:caption: Tools

tools/trustctl-menu
tools/trustctl-tray
tools/trustctl-import
tools/misc
```

```{toctree}
:hidden:
:caption: Security

security/model
security/physical-access
```

```{toctree}
:hidden:
:caption: More

troubleshooting
development/building
development/testing
development/display-images
development/releasing
license
```
