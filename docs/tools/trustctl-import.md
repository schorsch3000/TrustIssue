# trustctl-import (from Google Authenticator)

`trustctl-import` moves the accounts of a Google Authenticator export onto the device. It is its own
small static binary, next to `trustctl` in every release archive (`trustctl/cmd/trustctl-import`; built
with `tools/build/build_trustctl.sh`).

```sh
trustctl-import                              # finds trustctl next to itself, via $TRUSTCTL, or in the PATH
trustctl-import --trustctl ./trustctl -- --device /dev/ttyUSB0   # after -- come options for trustctl
trustctl-import --no-browser                 # only print the address to open
```

## How to use it

1. Run `trustctl-import`. It opens a page in your browser: a server on `127.0.0.1` on a random port that only
   that page can use (token, `Host` and `Origin` checked). Quit with the *Quit* button or Ctrl-C; it also
   ends after 30 minutes without use.
2. In Authenticator choose *Transfer accounts*, then *Export accounts*, and show the QR code to the
   webcam. Several codes (a split export) are read one after the other. The browser reads the camera and the
   QR code (the browser's `BarcodeDetector`, or the bundled jsQR).
3. The page lists the accounts with a checkbox and an editable name each. Untick what you do not need,
   correct the names (the device's name rules are checked as you type) and press *Import selected*.
4. Every key is stored with `trustctl add`, so **every key still needs its button press** on the device
   ({doc}`../reference/trustctl`, `add`). A failed key goes on to the next; a denial, a timeout or a missing
   device stops, and keys already stored are remembered so a retry cannot store them twice.

Accounts the device cannot hold (HOTP, SHA-256 or SHA-512, secrets over 20 bytes) are listed greyed out
with the reason.

## Options

`--trustctl PATH`
: The `trustctl` binary (default: `$TRUSTCTL`, next to this program, or the `PATH`).

`--no-browser`
: Do not open a browser, only print the address.

`-- OPTIONS`
: Everything after `--` goes in front of `add`, for example `-- --device /dev/ttyUSB0`.

## What it does with the secrets

The secrets stay in the `trustctl-import` process and in the pipe to `trustctl` (as an `otpauth://` URI on
standard input, never a command line or a file). The browser page only ever gets names, ids and reasons.
An export QR code holds every secret in the clear: do this on a computer you trust and close the
Authenticator screen afterwards.
