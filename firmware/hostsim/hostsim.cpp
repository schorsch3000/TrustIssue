// Runs the real core firmware (core.ino, unchanged) on the host against mock Arduino, EEPROM and
// display headers, driven by a script on stdin. Used to produce the display pictures of the
// documentation (tools/docs/render_display.py); see README.md.
//
// stdout, one event per line:  <ms> F <contrast> <hex of 512 bytes>   the display changed
//                              <ms> > <text>                           the device sent a line
//                              <ms> < <text>                           the host sent a line
//                              <ms> M <text>                           a mark from the script
//                              <ms> B <1|0>                            the button went down / up

#include "hostsim.h"

#include <sys/mman.h>

#include <Arduino.h>

#include "../core/core.ino"

uint32_t hs_now_ms;
bool hs_button_down;
bool hs_pin_set = true;
bool hs_unlocked;
uint8_t hs_eeprom[1024];

// ---------------------------------------------------------------- display

static uint8_t frameBuffer[4][128];  // SSD1306 page layout: 8 pixels per byte, bit 0 on top
static bool invertOn, flipOn, powerOn = true;
static uint8_t contrastValue = 0x7F;
static bool recording = true;
static char lastFrame[1100];

void hs_display_tile(uint8_t x, uint8_t y, const uint8_t *tile) {
  if (x < 16 && y < 4) memcpy(&frameBuffer[y][x * 8], tile, 8);
}
void hs_display_contrast(uint8_t value) { contrastValue = value; }
void hs_display_power(bool on) { powerOn = on; }
void hs_display_invert(bool on) { invertOn = on; }
void hs_display_flip(bool on) { flipOn = on; }

static bool pixel(int x, int y) { return frameBuffer[y / 8][x] >> (y % 8) & 1; }

// What a person looking at the display sees (rotation and inversion applied by the controller).
void hs_snapshot() {
  if (!recording) return;
  char text[1100];
  int n = snprintf(text, sizeof(text), "%u ", powerOn ? contrastValue : 0xFFFFu);
  for (int page = 0; page < 4; page++) {
    for (int x = 0; x < 128; x++) {
      uint8_t byte = 0;
      for (int bit = 0; bit < 8; bit++) {
        int sx = x, sy = page * 8 + bit;
        if (flipOn) sx = 127 - sx, sy = 31 - sy;
        bool lit = powerOn && (pixel(sx, sy) != invertOn);
        byte |= lit << bit;
      }
      n += snprintf(text + n, sizeof(text) - n, "%02x", byte);
    }
  }
  if (strcmp(text, lastFrame) == 0) return;
  strcpy(lastFrame, text);
  printf("%u F %s\n", hs_now_ms, text);
}

void hs_advance(uint32_t ms) {
  hs_now_ms += ms;
  hs_snapshot();
}

// ---------------------------------------------------------------- serial

static char serialIn[8192];
static unsigned inHead, inTail;
static char outLine[256];
static unsigned outLength;

void hs_serial_out(char c) {
  if (c == '\n') {
    outLine[outLength] = '\0';
    if (recording || 1) printf("%u > %s\n", hs_now_ms, outLine);
    outLength = 0;
  } else if (outLength < sizeof(outLine) - 1) {
    outLine[outLength++] = c;
  }
}
bool hs_serial_avail() { return inHead != inTail; }
char hs_serial_in() { return serialIn[inTail++ % sizeof(serialIn)]; }

static void feed(const char *line) {
  printf("%u < %s\n", hs_now_ms, line);
  for (const char *p = line; *p; p++) serialIn[inHead++ % sizeof(serialIn)] = *p;
  serialIn[inHead++ % sizeof(serialIn)] = '\n';
}

// ---------------------------------------------------------------- driver

static void button(bool down) {
  if (hs_button_down == down) return;
  hs_button_down = down;
  printf("%u B %d\n", hs_now_ms, down);
}

static void wait(unsigned ms) {
  for (unsigned i = 0; i < ms; i++) {
    hs_now_ms++;
    loop();
    hs_snapshot();
  }
}

static bool readFile(const char *path, uint8_t *dst, size_t size) {
  FILE *f = fopen(path, "rb");
  if (!f) return false;
  size_t got = fread(dst, 1, size, f);
  fclose(f);
  return got == size;
}

int main() {
  memset(hs_eeprom, 0xFF, sizeof(hs_eeprom));
  // PARAMS lives in read-only memory; make its pages writable so a script can patch a font or logo in.
  long pageSize = 4096;
  uintptr_t start = (uintptr_t)PARAMS & ~(uintptr_t)(pageSize - 1);
  mprotect((void *)start, ((uintptr_t)PARAMS + sizeof(PARAMS) - start + pageSize - 1) & ~(uintptr_t)(pageSize - 1), PROT_READ | PROT_WRITE);
  uint8_t *params = (uint8_t *)PARAMS;

  char command[512];
  while (fgets(command, sizeof(command), stdin)) {
    command[strcspn(command, "\r\n")] = '\0';
    char *arg = strchr(command, ' ');
    if (arg) *arg++ = '\0';
    else arg = (char *)"";
    if (!*command || *command == '#') continue;
    if (!strcmp(command, "boot")) {
      setup();
      hs_snapshot();
    } else if (!strcmp(command, "serial")) feed(arg);
    else if (!strcmp(command, "wait")) wait(atoi(arg));
    else if (!strcmp(command, "press")) button(true);
    else if (!strcmp(command, "release")) button(false);
    else if (!strcmp(command, "hold")) {
      button(true);
      wait(atoi(arg));
      button(false);
    } else if (!strcmp(command, "tap")) {  // press, 150 ms, release, then the given pause (default 300 ms)
      button(true);
      wait(150);
      button(false);
      wait(*arg ? atoi(arg) : 300);
    } else if (!strcmp(command, "record")) {  // "record on" starts a clip with the display as it is now
      recording = !strcmp(arg, "on");
      if (recording) {
        lastFrame[0] = '\0';
        printf("%u M record\n", hs_now_ms);
        hs_snapshot();
      }
    }
    else if (!strcmp(command, "mark")) printf("%u M %s\n", hs_now_ms, arg);
    else if (!strcmp(command, "pin")) hs_pin_set = !strcmp(arg, "set");
    else if (!strcmp(command, "unlocked")) hs_unlocked = !strcmp(arg, "on");
    else if (!strcmp(command, "logo")) {
      if (!readFile(arg, params + 0x314, 512)) { fprintf(stderr, "cannot read logo %s\n", arg); return 1; }
      params[5] |= 2;
    } else if (!strcmp(command, "font")) {
      if (!readFile(arg, params + 0x10, 772)) { fprintf(stderr, "cannot read font %s\n", arg); return 1; }
      params[5] |= 1;
    } else if (!strcmp(command, "eeprom_save") || !strcmp(command, "eeprom_load")) {  // keys etc. between runs
      bool save = !strcmp(command, "eeprom_save");
      FILE *f = fopen(arg, save ? "wb" : "rb");
      if (!f || (save ? fwrite(hs_eeprom, 1, sizeof(hs_eeprom), f) : fread(hs_eeprom, 1, sizeof(hs_eeprom), f)) != sizeof(hs_eeprom)) {
        fprintf(stderr, "cannot %s %s\n", command, arg);
        return 1;
      }
      fclose(f);
    } else if (!strcmp(command, "contrast")) {  // full, dim
      params[6] = (uint8_t)strtol(arg, &arg, 0);
      params[7] = (uint8_t)strtol(arg, NULL, 0);
    } else {
      fprintf(stderr, "unknown script command: %s\n", command);
      return 1;
    }
    fflush(stdout);
  }
  return 0;
}
