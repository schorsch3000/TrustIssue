# Security model

## What it protects against

The main threat is a **compromised host**: malware on the laptop. It controls the serial port, the reset
line and the timing. Against it the device guarantees:

- **Secrets can never be exported.** No command returns a stored secret, and no firmware that the host
  can flash reads them out, because...
- **...the host cannot reflash the device.** The bootloader only opens after a gesture on the button
  (and the update PIN, if set). Holding the button through a reset, the "bait and switch" in which
  malware resets the chip while you hold the button for a real request, never opens it.
- **Every use needs a human.** Adding, deleting, wiping, setting the timezone, changing the settings and
  every code request show what they are about on the display and need a deliberate 1 to 2 second hold
  (wipe: 5 to 6 seconds). Malware cannot silently delete keys or silently ask for codes.
- **The time is checked by you.** The confirm screen shows the local date and time the code is for,
  computed on the device. The timezone offset is stored on the device and changed only by a confirmed
  command, never in the same request as the time, so a host cannot ask for a code valid tomorrow and
  have it look right.
- **One request at a time.** While a confirmation is pending, other commands get `ERR BUSY`.

## What it does not protect against

- **TOTP stays phishable.** The code passes through the host. A phishing site that gets you to type a
  code gets the code. The gain is that the secrets can never be exported and each code needs a human press.
  Use FIDO2 where a service offers it.
- **A host that tricks you.** The display shows what is being requested; if you hold the button without
  reading, you approve. Read the name and the time.
- **Physical attacks on the chip itself**, see {doc}`physical-access`.

## The design in a few lines

- The button is debounced in software and the gesture is *release inside a window*: 1 to 2 s, wipe 5 to
  6 s, autonomous wipe 10 to 11 s. Releasing before or after is a denial; holding past the end denies at
  once. The window is shown on the bar as a checkerboard.
- A press that was already going when a request arrived does not count.
- Delete zeroes the secret, so unused slots never hold key material. Wipe overwrites all secrets
  interleaved across the slots, so an interrupted wipe leaves every key equally damaged.
- There is no `update` command: change a key by deleting and adding it.
- Names are validated by the device itself (ASCII only, no leading, trailing or double spaces, unique
  ignoring case) and never truncated.
- Backups: keep the original secrets or QR codes offline. There is no export by design.

For what an attacker with the device in their hands can do, see {doc}`physical-access`.
