# Third-party licenses

Trust Issues itself is free software under the GNU General Public License, version 3 or (at your option) any later
version (see `LICENSE`), Copyright (C) 2026 Dirk Heilig. It contains, links or ships the following third-party
material. Each part keeps its own license; all of them are compatible with the GPL.

Binary releases of `trustctl` embed the compiled core and calibration firmware (`trustctl/internal/firmware`), so the
notices for the firmware's libraries apply to the `trustctl` binaries too, and the notices for the Go libraries apply
to the binaries as well.

## Overview

| Part                                                                                           | Used in                                                                | License                              |
| ---------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- | ------------------------------------ |
| Arduino AVR core, `Wire`, `EEPROM` (Arduino LLC and others)                                    | core and calibration firmware                                          | LGPL 2.1 or later                    |
| avr-libc                                                                                       | all firmware, bootloader                                               | modified BSD (3 clause)              |
| U8g2 / U8x8 (Oliver Kraus)                                                                     | calibration firmware only                                              | BSD 2 clause (code); fonts see below |
| `go.bug.st/serial` (Cristian Maglie)                                                           | `trustctl`                                                             | BSD 3 clause                         |
| Go standard library, `golang.org/x/sys`, `golang.org/x/term` (The Go Authors)                  | `trustctl`                                                             | BSD 3 clause                         |
| Victoria, Chroma48, Artos, Torus fonts (U8g2 font group "Open Game Art", from opengameart.org) | font data in the bootloader, the device parameter block and `trustctl` | CC0 1.0 (public domain)              |
| X11 "misc fixed" 5x8                                                                           | font data in `trustctl`                                                | public domain                        |

## Arduino core, Wire, EEPROM (LGPL 2.1 or later)

The core firmware and the calibration firmware are compiled against the Arduino AVR core (version 1.8.8) with its
`Wire` and `EEPROM` libraries, licensed under the GNU Lesser General Public License, version 2.1, "or (at your option)
any later version" (checked in the headers of the installed package). The license text is at
https://www.gnu.org/licenses/old-licenses/lgpl-2.1.txt and in the Arduino package. The LGPL lets this project combine
the library with GPL code under the GPL version 3. The complete source of the firmware and the build scripts
(`firmware/core/build.sh`, `firmware/calibration/build.sh`) are in this repository, so anyone can rebuild the firmware
against a modified version of the library, which is what the LGPL asks for.

## avr-libc (modified BSD)

The firmware is linked against avr-libc (https://github.com/avrdudes/avr-libc), Copyright (c) the avr-libc authors,
under the following terms (the full list of copyright holders is in avr-libc's own `LICENSE`):

> Redistribution and use in source and binary forms, with or without modification, are permitted provided that the
> following conditions are met:
>
> 1. Redistributions of source code must retain the above copyright notice, this list of conditions and the following
>    disclaimer.
> 2. Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the
>    following disclaimer in the documentation and/or other materials provided with the distribution.
> 3. Neither the name of the copyright holders nor the names of its contributors may be used to endorse or promote
>    products derived from this software without specific prior written permission.
>
> THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES,
> INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
> DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDERS OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
> SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
> SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY,
> WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
> OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

## U8g2 / U8x8 (BSD 2 clause), calibration firmware only

The calibration firmware (`firmware/calibration`) uses the U8x8 part of U8g2 (https://github.com/olikraus/u8g2). The
core firmware and the bootloader do not contain any U8g2 code.

```
Copyright (c) 2016, olikraus@gmail.
All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

* Redistributions of source code must retain the above copyright notice, this list
of conditions and the following disclaimer.

* Redistributions in binary form must reproduce the above copyright notice, this
list of conditions and the following disclaimer in the documentation and/or other
materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND
CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES,
INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR
CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT
NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF
ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

U8g2's own `LICENSE` says that its fonts are licensed under different conditions, per font group
(https://github.com/olikraus/u8g2/wiki/fntgrp). The calibration firmware contains only the `chroma48medium8` font
(group "Open Game Art", CC0). Earlier builds also carried fonts from groups with other terms (Press Start 2P, PC Senior,
PxPlus IBM CGA, Amstrad CPC extended); they were removed so that nothing with a restrictive license is redistributed.

## Fonts (CC0 and public domain)

`firmware/fonts/victoriamedium8.bin`, the glyph data in the bootloader (`firmware/bootloader/boot_font.h`, generated
from it), the font in the device parameter block and the seven fonts in `trustctl/internal/fonts` come from U8g2's
font collection. Six of them (victoriamedium8, victoriabold8, chroma48medium8, artossans8, artosserif8,
torussansbold8) are from "A package of 8-bit fonts for Grafx2 and Linux" on opengameart.org, marked CC0 1.0 / public
domain (https://creativecommons.org/publicdomain/zero/1.0/); `5x8` is the X11 "misc fixed" font, public domain. Details
in `trustctl/internal/fonts/README.md`.

## Go libraries in `trustctl` (BSD 3 clause)

`go.bug.st/serial` v1.8.0:

```
Copyright (c) 2014-2026, Cristian Maglie.
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions
are met:

1. Redistributions of source code must retain the above copyright
   notice, this list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright
   notice, this list of conditions and the following disclaimer in
   the documentation and/or other materials provided with the
   distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived
   from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS
FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE
COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT,
INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING,
BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT
LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN
ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.
```

The Go standard library, `golang.org/x/sys` and `golang.org/x/term`:

```
Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## Not covered

- `Demo-Logos/` (the demo logos) are AI generated and refined by the author. They fall under the project license
  (GPL-3.0-or-later). Note that AI generated images may not be copyrightable in every jurisdiction.
- `Case/` (3D models of a case) is work in progress and not committed yet. It will be added under the project license
  once finished. The case files include third-party models (the Arduino Nano, a push button and an SSD1306 display)
  whose licenses are those of their authors; this project does not relicense them.
