// Runs the real bootloader (firmware/bootloader/boot.c, only its two assembler lines replaced) on the
// host against fake registers and an SSD1306 emulation, along a timeline from a script on stdin.
// Same output as hostsim (<ms> F <contrast> <hex>, <ms> B <1|0>, <ms> M record), plus
//   <ms> E <APP|RESET|LIMIT>   how it ended: application started, watchdog reset, time limit of the script
//   <ms> U <hex>               bytes the bootloader sent on the serial port
//
// Script (one command per line, times in ms since reset):
//   at MS press|release     the button
//   at MS cap TEXT          a caption for the picture from then on (a "M cap" event)
//   at MS uart TEXT         the host sends TEXT (\n and \x02 escapes) at that time
//   pin set|none            is there an update PIN (the page at 0x7780)
//   firmware present|absent
//   config HEX              configuration byte of the EEPROM header (bit 0 rotate, bit 1 invert)
//   limit MS                stop here (default 60000)
//   run

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <avr/io.h>

static void runApp();
#define hs_run_app() runApp()

// ---------------------------------------------------------------- time

static uint64_t nowUs;
static uint64_t limitMs = 60000;
static bool wdtOn;
static uint64_t wdtDeadlineUs;
static unsigned wdtMs;

struct ButtonEvent { uint64_t ms; bool down; char caption[96]; };  // a caption has no button action
static ButtonEvent buttonEvents[256];
static int buttonCount, buttonDone;
static bool buttonDown;
struct UartByte { uint64_t ms; uint8_t value; };
static UartByte uartIn[512];
static int uartCount, uartTaken;

static void finish(const char *how) {
  printf("%llu E %s\n", (unsigned long long)(nowUs / 1000), how);
  fflush(stdout);
  exit(0);
}

static void flushButton() {  // report button changes at their scheduled times
  while (buttonDone < buttonCount && buttonEvents[buttonDone].ms * 1000 <= nowUs) {
    ButtonEvent &e = buttonEvents[buttonDone++];
    if (e.caption[0]) {
      printf("%llu M cap %s\n", (unsigned long long)e.ms, e.caption);
    } else {
      buttonDown = e.down;
      printf("%llu B %d\n", (unsigned long long)e.ms, buttonDown);
    }
  }
}

static void advance(uint64_t us) {
  nowUs += us;
  flushButton();
  if (wdtOn && nowUs > wdtDeadlineUs) finish("RESET");
  if (nowUs / 1000 > limitMs) finish("LIMIT");
}

void hs_delay_ms(unsigned ms) { advance((uint64_t)ms * 1000); }
void hs_wdt_enable(unsigned ms) {
  if (ms <= 15) finish("RESET");  // reboot(): the chip restarts at once
  wdtOn = true;
  wdtMs = ms;
  wdtDeadlineUs = nowUs + (uint64_t)ms * 1000;
}
void hs_wdt_disable() { wdtOn = false; }
void hs_wdt_reset() { wdtDeadlineUs = nowUs + (uint64_t)wdtMs * 1000; }
static void runApp() { finish("APP"); }

// ---------------------------------------------------------------- flash, EEPROM

static bool pinSet, firmwarePresent = true;
static uint8_t configByte;

uint8_t hs_flash_byte(uint16_t address) {
  if (address >= 0x7780 && address < 0x7800) return pinSet ? (address - 0x7780 < 4 ? "4711"[address - 0x7780] : 0xFF) : 0xFF;
  return 0xFF;
}
uint16_t hs_flash_word(uint16_t address) { return address == 0 && firmwarePresent ? 0x940C : 0xFFFF; }

uint8_t hs_PORTC, hs_MCUSR, hs_EECR;
uint16_t hs_EEAR;
HsReg hs_TWCR(0), hs_TWDR(1), hs_TWSR(2), hs_TWBR(3), hs_UCSR0A(4), hs_UCSR0B(5), hs_UDR0(6), hs_UBRR0L(7),
    hs_PINC(8), hs_EEDR(9);

// ---------------------------------------------------------------- the display (SSD1306, page addressing)

static uint8_t frameBuffer[4][128];
static bool powerOn, invertOn, flipOn;  // a reset SSD1306 is off until 0xAF
static uint8_t contrast = 0x7F;
static uint8_t page, column;
static char lastFrame[1100];

static void snapshot() {
  char text[1100];
  int n = snprintf(text, sizeof(text), "%u ", powerOn ? contrast : 0xFFFFu);
  for (int p = 0; p < 4; p++) {
    for (int x = 0; x < 128; x++) {
      uint8_t byte = 0;
      for (int bit = 0; bit < 8; bit++) {
        int sx = x, sy = p * 8 + bit;
        if (flipOn) sx = 127 - sx, sy = 31 - sy;
        bool lit = powerOn && ((frameBuffer[sy / 8][sx] >> (sy % 8) & 1) != invertOn);
        byte |= lit << bit;
      }
      n += snprintf(text + n, sizeof(text) - n, "%02x", byte);
    }
  }
  if (strcmp(text, lastFrame) == 0) return;
  strcpy(lastFrame, text);
  printf("%llu F %s\n", (unsigned long long)(nowUs / 1000), text);
}

static bool nextIsAddress;
static int pendingArgs;  // bytes still to swallow as arguments of a two-byte command
static uint8_t pendingCommand;

static void command(uint8_t c) {
  if (pendingArgs) {
    if (pendingCommand == 0x81) contrast = c;
    pendingArgs--;
    return;
  }
  if ((c & 0xF8) == 0xB0) page = c & 7;
  else if ((c & 0xF0) == 0x00) column = (column & 0xF0) | (c & 0x0F);
  else if ((c & 0xF0) == 0x10) column = (column & 0x0F) | (uint8_t)((c & 0x0F) << 4);
  else if (c == 0xA0) flipOn = true;
  else if (c == 0xA1) flipOn = false;
  else if (c == 0xA6) invertOn = false;
  else if (c == 0xA7) invertOn = true;
  else if (c == 0xAE) powerOn = false;
  else if (c == 0xAF) powerOn = true;
  else if (c == 0x81 || c == 0x20 || c == 0xD5 || c == 0xA8 || c == 0xD3 || c == 0x8D || c == 0xDA || c == 0xD9 ||
           c == 0xDB) {
    pendingCommand = c;
    pendingArgs = 1;
  }
}

static uint8_t controlByte;
static bool haveControl;

static void twiByte(uint8_t value) {
  advance(25);  // 9 bits at 400 kHz
  if (nextIsAddress) {
    nextIsAddress = false;
    haveControl = false;
    return;
  }
  if (!haveControl) {
    controlByte = value;
    haveControl = true;
    return;
  }
  if (controlByte == 0x00) {
    command(value);
  } else {
    frameBuffer[page & 3][column & 127] = value;
    column = (column + 1) & 127;
  }
}

// ---------------------------------------------------------------- registers

static uint8_t twdr;

static bool uartReady() { return uartTaken < uartCount && uartIn[uartTaken].ms * 1000 <= nowUs; }

HsReg &HsReg::operator=(uint8_t value) {
  switch (id) {
    case 0:  // TWCR: a start condition, a stop, or sending the byte in TWDR
      if (value & _BV(TWSTA)) {
        nextIsAddress = true;
        haveControl = false;
      } else if (value & _BV(TWSTO)) {
        snapshot();
      } else {
        twiByte(twdr);
      }
      break;
    case 1: twdr = value; break;
    case 6:
      printf("%llu U %02x\n", (unsigned long long)(nowUs / 1000), value);
      break;
    default: break;
  }
  return *this;
}

HsReg::operator uint8_t() const {
  switch (id) {
    case 0: return _BV(TWINT) | _BV(TWEN);  // always done, a stop is immediate
    case 2: return 0x18;                    // address acknowledged
    case 4:                                 // UCSR0A: polling takes time
      advance(50);
      return _BV(UDRE0) | (uartReady() ? _BV(RXC0) : 0);
    case 6: return uartReady() ? uartIn[uartTaken++].value : 0;
    case 8:
      flushButton();
      return buttonDown ? 0xF7 : 0xFF;  // PC3 low while pressed
    case 9: return hs_EEAR == 0 ? 0xA1 : hs_EEAR == 2 ? configByte : 0;
    default: return 0;
  }
}

// ---------------------------------------------------------------- the bootloader itself

#define main boot_main
#include "boot_patched.c"
#undef main

static void unescape(const char *text) {
  for (const char *p = text; *p; p++) {
    uint8_t value = *p;
    if (*p == '\\' && p[1] == 'n') value = '\n', p++;
    else if (*p == '\\' && p[1] == 'x') value = (uint8_t)strtol(p + 2, NULL, 16), p += 3;
    uartIn[uartCount].value = value;
    uartCount++;
  }
}

int main() {
  char line[512];
  while (fgets(line, sizeof(line), stdin)) {
    line[strcspn(line, "\r\n")] = '\0';
    char *arg = strchr(line, ' ');
    if (arg) *arg++ = '\0';
    else arg = (char *)"";
    if (!*line || *line == '#') continue;
    if (!strcmp(line, "at")) {
      uint64_t ms = strtoull(arg, &arg, 10);
      while (*arg == ' ') arg++;
      if (!strncmp(arg, "press", 5) || !strncmp(arg, "release", 7)) {
        buttonEvents[buttonCount].ms = ms;
        buttonEvents[buttonCount++].down = !strncmp(arg, "press", 5);
      } else if (!strncmp(arg, "cap ", 4)) {  // a caption for the picture, shown from this time on
        buttonEvents[buttonCount].ms = ms;
        strncpy(buttonEvents[buttonCount++].caption, arg + 4, 95);
      } else if (!strncmp(arg, "uart ", 5)) {
        int first = uartCount;
        unescape(arg + 5);
        for (int i = first; i < uartCount; i++) uartIn[i].ms = ms;
      } else {
        fprintf(stderr, "bad at command: %s\n", arg);
        return 1;
      }
    } else if (!strcmp(line, "pin")) pinSet = !strcmp(arg, "set");
    else if (!strcmp(line, "firmware")) firmwarePresent = !strcmp(arg, "present");
    else if (!strcmp(line, "config")) configByte = (uint8_t)strtol(arg, NULL, 16);
    else if (!strcmp(line, "limit")) limitMs = strtoull(arg, NULL, 10);
    else if (!strcmp(line, "run")) {
      printf("0 M record\n");
      boot_main();
      finish("APP");
    } else {
      fprintf(stderr, "unknown script command: %s\n", line);
      return 1;
    }
  }
  return 0;
}
