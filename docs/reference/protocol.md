# Serial protocol

`trustctl` speaks a line based ASCII protocol with the device over the USB serial port, 9600 baud, 8N1.
You only need this page to write your own client. The protocol is strictly request and response: the
host sends the next command only after the final response of the previous one. The same protocol is
spoken between `trustctl` and its daemon over the Unix socket.

Fixed fields come first and the free text name comes last, so spaces in names need no escaping. Lines end
with `\n` (a `\r` is ignored); a line can be at most 99 characters.

## Commands

```text
HELLO                                  -> TrustIssue 0.2.1 proto 1 caps=autowipe nopin unlocked
LIST                                   -> KEY <id> <name> ... END
ADD <base32> <algo> <digits> <period> <name...>
                                       -> PENDING ... OK <id> | ERR <code>
DEL <id>                               -> PENDING ... OK | ERR <code>
TOKEN <id> <unixtime>                  -> PENDING ... OK 012345 | ERR <code>
GETTZ                                  -> OK <quarter-hours>
SETTZ <quarter-hours>                  -> PENDING ... OK | ERR <code>
GETCFG                                 -> OK <flags-hex>
SETCFG <flags-hex>                     -> PENDING ... OK | ERR <code>
WIPE                                   -> PENDING ... OK | ERR <code>
```

- `HELLO` is also sent unprompted when the device starts. After the version and `proto 1` come the
  capability and warning words: `caps=autowipe[,...]`, `nopin` (no update PIN set), `unlocked` (lock bits
  or fuses not final).
- `ADD`: `<algo>` is `SHA1`; `<digits>` 6 to 8; `<period>` 30 or 60; `<base32>` at most 20 bytes decoded.
- `TOKEN`: `<unixtime>` is an unsigned 32 bit number (the host's clock). The code is a string with leading
  zeros.
- `GETTZ` and `SETTZ`: a signed offset in 15 minute units (UTC-12 is -48, UTC+14 is 56).
- `GETCFG` and `SETCFG`: a two digit hex byte, bit 0 rotate, 1 invert, 2 logo, 3 autowipe, 4 show-version.
- `LIST` answers one `KEY` line per used slot, then `END`. The id is the slot index.

Commands that need the button are answered with `PENDING` first. Validation errors (`DUPLICATE`, `BADARG`,
`NOTFOUND`, `FULL`) come at once, without `PENDING` and without a prompt; after `PENDING` comes `OK ...` or
`ERR ...` when the person decided. `WIPE` on an empty device is answered `OK` immediately.

While a request is pending, **every other line, even `HELLO`**, gets `ERR BUSY`. A pending request ends
with `ERR TIMEOUT` after 30 seconds.

## Error codes

| Code        | Meaning                                                                     |
| ----------- | --------------------------------------------------------------------------- |
| `DENIED`    | refused: released too early or too late, or held past the window            |
| `TIMEOUT`   | nobody answered in time                                                     |
| `BUSY`      | another request is pending (or the autonomous wipe is running)              |
| `FULL`      | all 20 slots are in use                                                     |
| `DUPLICATE` | a name that equals an existing one, ignoring case                           |
| `BADARG`    | malformed or invalid arguments                                              |
| `NOTFOUND`  | no key with that id                                                         |
| `UNKNOWN`   | no such command                                                             |

## An example

```text
< ADD JBSWY3DPEHPK3PXP SHA1 6 30 GitHub
> PENDING
  (the person holds the button for 1.5 s)
> OK 0
< TOKEN 0 1792316460
> PENDING
> OK 885822
```

## Notes for client authors

- Opening the port resets the board on DTR (on most boards): ask `HELLO` and wait for an answer or the
  boot banner, allow 1 to 2 seconds.
- The Arduino's receive buffer is 128 bytes and bytes that arrive while it is full are lost: there is no
  flow control. An `ADD` line is up to about 75 bytes; do not send the next command before the
  response.
- Never send a timezone and a time as one request: the device deliberately keeps the offset separate.
- No command ever returns a stored secret.
