// Trust Issues bootloader for the ATmega328P at 16 MHz, 2 KB boot section.
//
// Normal start: button (D2, to GND) not held and a firmware present: jump to
// the application at once. The bootloader does not listen on serial then.
//
// Button held at reset: the update gate. The host cannot press the button, so
// it cannot open the gate, however it resets the chip or talks to the port:
//   1. "RELEASE BUTTON" (inverted) until the button is up. Holding through a
//      reset (a real token request interrupted by malware) goes no further.
//   2. "FIRMWARE UPDATE" and a bar that drains over 4 s; pressing the button
//      during that time aborts and resets.
//   3. Gesture: tap, tap, then press again and hold for 2 s; the bottom row
//      lights up one step per press.
//   4. If an update PIN is set (the last page of the application area), the host
//      has to send it; a wrong PIN is only noticed after a 7 s bar, then the
//      application starts. See pin_gate().
//   5. Update mode: a small subset of STK500v1 (sync, load address, program page
//      with a checksum byte and an in-place verify, read signature, leave), as
//      spoken by trustctl and tools/build/flash.py. Flash can be written but never
//      read back (the PIN lives there), the EEPROM is neither readable nor
//      writable, and the bootloader's own section is protected.
// Every timeout or mistake ends in a reset, which starts the application.
// A missing or dead display fails closed: the update mode never opens.

#include <avr/boot.h>
#include <avr/io.h>
#include <avr/pgmspace.h>
#include <avr/wdt.h>
#include <stdbool.h>
#include <stdint.h>
#include <util/delay.h>

#include "boot_font.h"  // generated: CHARSET, GLYPHS, CHARSET_SIZE

#define BOOT_START 0x7800UL
#define PAGE_BYTES SPM_PAGESIZE
#define BUTTON_BIT PD2
#define OLED_ADDRESS 0x3C
#define OLED_CONTRAST 0x20

#define STK_OK 0x10
#define STK_FAILED 0x11
#define STK_INSYNC 0x14
#define STK_NOSYNC 0x15
#define CRC_EOP 0x20

// The optional update PIN: the last page of the application area. Up to 15
// characters followed by 0xFF up to the end of the page; a blank page (0xFF first) means no PIN is set.
#define PIN_ADDRESS 0x7780U
#define PIN_MAX 15
#define PIN_START 0x02   // the host's PIN line is STX, the characters, then LF
#define PIN_PROMPT '?'   // sent every 100 ms until the host answers
#define CHECK_TICKS 5    // 128 columns x 5 ticks x 10 ms: the bar drains in about 7 s

// ---- messages (at most 16 characters; boot_font.h carries exactly these glyphs)

static const char M_RELEASE[] PROGMEM = "RELEASE";
static const char M_BUTTON[] PROGMEM = "BUTTON";
static const char M_UPDATE[] PROGMEM = "FIRMWARE UPDATE";
static const char M_WAIT[] PROGMEM = "WAIT";
static const char M_TAPS[] PROGMEM = "TAP TAP HOLD";
static const char M_MODE[] PROGMEM = "UPDATE MODE";
static const char M_PIN[] PROGMEM = "PIN";

// ---- start

int main(void) __attribute__((used));
void start(void) __attribute__((naked, section(".vectors"), used));
void start(void) { __asm__ volatile("clr r1\n\trjmp main"); }

// ---- reset helpers

static void run_app(void) __attribute__((noreturn));
static void run_app(void) {
  wdt_disable();  // the application sets up everything else itself
  __asm__ volatile("jmp 0");
  for (;;) {}
}

// A watchdog reset: the chip restarts, and without the button held that
// starts the application.
static void reboot(void) __attribute__((noreturn));
static void reboot(void) {
  wdt_enable(WDTO_15MS);
  for (;;) {}
}

// ---- button

static bool button(void) { return !(PIND & _BV(BUTTON_BIT)); }

static void tick(void) {
  _delay_ms(10);
  wdt_reset();
}

// Waits until the button has been in the wanted state for 3 ticks in a row.
// Returns the ticks waited, or -1 if max ticks (10 ms each) passed first.
static int16_t wait_level(bool down, uint16_t max) {
  uint8_t steady = 0;
  uint16_t waited = 0;
  while (waited < max) {
    tick();
    waited++;
    if (button() == down) {
      if (++steady >= 3) return waited;
    } else {
      steady = 0;
    }
  }
  return -1;
}

#define LONG_WAIT 1500  // 15 s

// ---- display (SSD1306 128x32 over the hardware TWI, page addressing)

static bool twi_start(void) {
  TWCR = _BV(TWINT) | _BV(TWSTA) | _BV(TWEN);
  while (!(TWCR & _BV(TWINT))) {}
  TWDR = OLED_ADDRESS << 1;
  TWCR = _BV(TWINT) | _BV(TWEN);
  while (!(TWCR & _BV(TWINT))) {}
  return (TWSR & 0xF8) == 0x18;
}

static void twi_write(uint8_t value) {
  TWDR = value;
  TWCR = _BV(TWINT) | _BV(TWEN);
  while (!(TWCR & _BV(TWINT))) {}
}

static void twi_stop(void) {
  TWCR = _BV(TWINT) | _BV(TWEN) | _BV(TWSTO);
  while (TWCR & _BV(TWSTO)) {}
}

static void oled_commands(const uint8_t *commands, uint8_t count, bool flash) {
  twi_start();
  twi_write(0x00);
  while (count--) twi_write(flash ? pgm_read_byte(commands++) : *commands++);
  twi_stop();
}

static void oled_put_glyph(char c, bool invert) {
  uint8_t index = 0;  // gen_font.py puts every character of the messages into CHARSET
  while (pgm_read_byte(&CHARSET[index]) != c) index++;
  for (uint8_t column = 0; column < 8; column++) {
    uint8_t bits = pgm_read_byte(&GLYPHS[index * 8 + column]);
    twi_write(invert ? ~bits : bits);
  }
}

// Starts the data stream of one display row (page).
static void oled_row_begin(uint8_t row) {
  uint8_t position[] = {0xB0 | row, 0x00, 0x10};
  oled_commands(position, 3, false);
  twi_start();
  twi_write(0x40);
}

// One text row (16 characters, msg in flash).
static void oled_row(uint8_t row, const char *msg, bool invert) {
  oled_row_begin(row);
  uint8_t i = 0;
  if (msg) {
    for (; i < 16; i++) {
      char c = pgm_read_byte(&msg[i]);
      if (!c) break;
      oled_put_glyph(c, invert);
    }
  }
  for (; i < 16; i++) oled_put_glyph(' ', invert);
  twi_stop();
}

// The bottom row as a bar: the first `pixels` columns are lit.
static uint8_t bar_shown __attribute__((section(".noinit")));
static void oled_bar(uint8_t pixels) {
  bar_shown = pixels;
  oled_row_begin(3);
  for (uint8_t i = 0; i < 128; i++) twi_write(i < pixels ? 0xFF : 0x00);
  twi_stop();
}

// Moves the bar to a new length in quick steps of 3 columns (a jump looks rough).
static void oled_bar_to(uint8_t target) {
  while (bar_shown != target) {
    uint8_t distance = bar_shown < target ? target - bar_shown : bar_shown - target;
    if (distance > 3) distance = 3;
    oled_bar(bar_shown < target ? bar_shown + distance : bar_shown - distance);
  }
}

static void show(const char *r0, const char *r1, const char *r2, const char *r3,
                 bool invert) {
  oled_row(0, r0, invert);
  oled_row(1, r1, invert);
  oled_row(2, r2, invert);
  oled_row(3, r3, invert);
}

// The configuration byte of the application's EEPROM header (rotation, inversion).
// Read only. A blank header gives 0; the CRC is not checked (it would cost
// bytes that do not fit): a damaged byte only rotates or inverts the prompts.
static uint8_t read_config(void) {
  uint8_t header[3];
  for (uint8_t i = 0; i < 3; i++) {
    while (EECR & _BV(EEPE)) {}
    EEAR = i;
    EECR |= _BV(EERE);
    header[i] = EEDR;
  }
  return header[0] == 0xA1 ? header[2] & 0x03 : 0;
}

static bool oled_init(uint8_t config) {
  TWSR = 0;
  TWBR = 12;  // 400 kHz
  if (!twi_start()) {
    twi_stop();
    return false;
  }
  twi_stop();
  static const uint8_t before[] PROGMEM = {0xAE, 0xD5, 0x80, 0xA8, 0x1F, 0xD3, 0x00,
                                           0x40, 0x8D, 0x14, 0x20, 0x02};
  static const uint8_t after[] PROGMEM = {0xDA, 0x02, 0x81, OLED_CONTRAST, 0xD9,
                                          0xF1, 0xDB, 0x40, 0x2E, 0xA4};
  oled_commands(before, sizeof(before), true);
  uint8_t rotate[] = {(config & 0x01) ? 0xA0 : 0xA1, (config & 0x01) ? 0xC0 : 0xC8};
  oled_commands(rotate, 2, false);
  oled_commands(after, sizeof(after), true);
  uint8_t last[] = {(config & 0x02) ? 0xA7 : 0xA6, 0xAF};
  oled_commands(last, 2, false);
  show(0, 0, 0, 0, false);
  return true;
}

// ---- the gate

static bool release_first(void) {
  show(0, M_RELEASE, M_BUTTON, 0, true);
  return wait_level(false, LONG_WAIT) >= 0;
}

// A bar in the bottom row that drains one column per `ticks` ticks. Pressing
// the button meanwhile (mashing) aborts: returns false.
static bool wait_bar(uint8_t ticks) {
  for (uint8_t pixels = 129; pixels--;) {
    oled_bar(pixels);
    if (wait_level(true, ticks) > 0) return false;
  }
  return true;
}

static bool gate(void) {
  if (button() && !release_first()) return false;

  show(M_UPDATE, M_WAIT, 0, 0, false);
  if (!wait_bar(3)) return false;  // about 4 s

  show(M_UPDATE, M_TAPS, 0, 0, false);
  for (;;) {  // tap, tap, then press and hold 2 s; every mistake starts over
    uint8_t taps = 0;
    while (taps < 2) {
      if (wait_level(true, taps ? 80 : LONG_WAIT) < 0) {  // next tap within 0.8 s
        if (!taps) return false;
        break;
      }
      if (wait_level(false, 60) < 0) {  // not released within 0.6 s: no tap
        if (wait_level(false, LONG_WAIT) < 0) return false;
        break;
      }
      oled_bar_to(++taps * 43);
    }
    if (taps < 2 || wait_level(true, 80) < 0) {
      oled_bar_to(0);
      continue;
    }
    bool early = false;  // the bar fills up while the third press is held (about 1.9 s)
    for (uint8_t held = 0; held < 43 && !early; held++) {
      oled_bar(86 + held);
      early = wait_level(false, 4) >= 0;  // released too early
    }
    if (!early) return release_first();
    oled_bar_to(0);
  }
}

// ---- STK500v1 programmer (flash only)

static uint8_t getch(void) {
  while (!(UCSR0A & _BV(RXC0))) {}
  wdt_reset();
  return UDR0;
}

static void putch(uint8_t value) {
  while (!(UCSR0A & _BV(UDRE0))) {}
  UDR0 = value;
}

static void verify_space(void) {
  if (getch() != CRC_EOP) reboot();
  putch(STK_INSYNC);
}

static void write_page(uint16_t address, const uint8_t *data, uint8_t length) {
  boot_page_erase(address);
  boot_spm_busy_wait();
  for (uint8_t i = 0; i < length; i += 2) {
    uint16_t word = data[i] | ((uint16_t)(i + 1 < length ? data[i + 1] : 0xFF) << 8);
    boot_page_fill(address + i, word);
  }
  boot_page_write(address);
  boot_spm_busy_wait();
  boot_rww_enable();
}

static void uart_on(void) {
  UCSR0A = _BV(U2X0);
  UBRR0L = 16;  // 115200 baud at 16 MHz
  UCSR0B = _BV(RXEN0) | _BV(TXEN0);
  wdt_enable(WDTO_8S);  // no activity for 8 s ends the PIN entry or the update mode
}

// The PIN gate, after the button gesture: with a PIN set, the host has to send
// it before the update mode opens. A wrong PIN looks exactly like a right one
// until the second wait bar has run out, so a guess costs the whole gate plus
// the bar however the attacker resets the chip. A wrong PIN just starts the
// application. The host can never read the PIN back: the update mode has no
// flash read-out and no compare command, and the page is only ever compared
// inside this function.
static bool pin_gate(void) {
  if (pgm_read_byte_near(PIN_ADDRESS) == 0xFF) return true;  // no PIN set

  show(M_UPDATE, M_PIN, 0, 0, false);
  uart_on();
  do {  // ask until the host answers
    putch(PIN_PROMPT);
    wait_level(true, 10);
  } while (!(UCSR0A & _BV(RXC0)));
  while (getch() != PIN_START) {}  // junk before the start byte is ignored
  uint8_t difference = 0;
  uint8_t position = 0;  // compared as the characters arrive; nothing is stored
  for (uint8_t c; (c = getch()) != '\n';) {
    difference |= c ^ pgm_read_byte_near(PIN_ADDRESS + position);
    if (position < PIN_MAX) position++;  // longer entries meet the 0xFF after the PIN
  }
  difference |= pgm_read_byte_near(PIN_ADDRESS + position) ^ 0xFF;  // the PIN ends here
  show(M_UPDATE, M_WAIT, 0, 0, false);  // the same screen as the first wait
  return wait_bar(CHECK_TICKS) && !difference;
}

static void update_mode(void) {
  static uint8_t buffer[PAGE_BYTES] __attribute__((section(".noinit")));
  uint16_t address = 0;

  show(0, M_MODE, 0, 0, false);
  uart_on();
  while (UCSR0A & _BV(RXC0)) (void)UDR0;  // polls sent while the PIN was checked

  for (;;) {
    uint8_t reply = STK_OK;
    uint8_t command = getch();
    switch (command) {
      case 0x55: {  // load address (word address)
        uint8_t low = getch();
        uint8_t high = getch();
        address = (((uint16_t)high << 8) | low) << 1;
        verify_space();
        break;
      }
      case 0x64: {  // program page
        uint16_t length = (uint16_t)getch() << 8;
        length |= getch();
        uint8_t type = getch();
        uint8_t sum = 0;
        for (uint16_t i = 0; i < length; i++) {
          uint8_t value = getch();
          sum += value;
          if (i < PAGE_BYTES) buffer[i] = value;
        }
        sum -= getch();  // the host appends the sum of the page: catches line errors
        verify_space();
        // flash only, whole page aligned, never the bootloader's own section
        if (type == 'F' && length <= PAGE_BYTES && !(address & (PAGE_BYTES - 1)) &&
            address + length <= BOOT_START && !sum) {
          write_page(address, buffer, length);
          // verify in place: flash is never read out, so nothing can be learned from it
          for (uint8_t i = 0; i < length; i++) {
            if (pgm_read_byte_near(address + i) != buffer[i]) reply = STK_FAILED;
          }
        } else {
          reply = STK_FAILED;
        }
        break;
      }
      case 0x75:  // read signature
        verify_space();
        putch(0x1E);
        putch(0x95);
        putch(0x0F);
        break;
      case 0x51:  // leave programming mode
        verify_space();
        putch(STK_OK);
        _delay_ms(10);
        reboot();
      default:  // get sync, enter progmode and anything else harmless
        verify_space();
        break;
    }
    putch(reply);
  }
}

// ---- main

int main(void) {
  MCUSR = 0;
  wdt_disable();
  PORTD |= _BV(BUTTON_BIT);  // pull-up
  _delay_ms(5);  // pull-up settled
  bool held = button();
  bool firmware = pgm_read_word(0) != 0xFFFF;

  if (!held && firmware) run_app();

  wdt_enable(WDTO_2S);  // a hung display bus must not hold the chip
  if (!oled_init(read_config())) {
    if (firmware) run_app();
    for (;;) {}  // no display, no firmware: nothing safe to do
  }
  if (gate() && pin_gate()) update_mode();
  reboot();
}
