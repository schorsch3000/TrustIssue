// Just enough of the Arduino API (and avr-libc's pgmspace) to build core.ino on the host.
#ifndef MOCK_ARDUINO_H
#define MOCK_ARDUINO_H

#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "../hostsim.h"

#define PROGMEM
#define PSTR(s) (s)
typedef const char *PGM_P;
class __FlashStringHelper;
#define F(s) ((const __FlashStringHelper *)(s))

#define min(a, b) ((a) < (b) ? (a) : (b))
#define max(a, b) ((a) > (b) ? (a) : (b))

#define HEX 16
#define INPUT_PULLUP 2
static const uint8_t A3 = 17;

inline uint32_t millis() { return hs_now_ms; }
inline void pinMode(uint8_t, uint8_t) {}
inline int digitalRead(uint8_t) { return hs_button_down ? 0 : 1; }

// A pointer reads the parameter block, a plain number reads the bootloader area's PIN page.
template <class T> inline uint8_t pgm_read_byte(const T *p) { return *(const volatile uint8_t *)p; }  // volatile: hostsim patches the parameter block at run time
inline uint8_t pgm_read_byte(unsigned int) { return hs_pin_set ? '4' : 0xFF; }

inline char *strncpy_P(char *dst, PGM_P src, size_t n) { return strncpy(dst, src, n); }
inline char *strcpy_P(char *dst, PGM_P src) { return strcpy(dst, src); }
// avr-libc's %S is a string in flash; here flash strings are plain strings.
inline int snprintf_P(char *dst, size_t size, PGM_P format, ...) {
  char fixed[64];
  size_t j = 0;
  for (size_t i = 0; format[i] && j < sizeof(fixed) - 1; i++) fixed[j++] = format[i] == 'S' && i && format[i - 1] == '%' ? 's' : format[i];
  fixed[j] = '\0';
  va_list args;
  va_start(args, format);
  int n = vsnprintf(dst, size, fixed, args);
  va_end(args);
  return n;
}

struct SerialClass {
  void begin(unsigned long) {}
  int available() { return hs_serial_avail(); }
  int read() { return hs_serial_in(); }
  void print(const char *s) { while (*s) hs_serial_out(*s++); }
  void print(const __FlashStringHelper *s) { print((const char *)s); }
  void print(char c) { hs_serial_out(c); }
  void print(int v, int base = 10) { printNumber((long)v, base); }
  void print(unsigned v, int base = 10) { printNumber((long)v, base); }
  void print(long v, int base = 10) { printNumber(v, base); }
  void print(unsigned long v, int base = 10) { printNumber((long)v, base); }
  void println() { hs_serial_out('\n'); }
  template <class T> void println(T v) { print(v); println(); }
  template <class T> void println(T v, int base) { print(v, base); println(); }

 private:
  void printNumber(long v, int base) {
    char buffer[24];
    snprintf(buffer, sizeof(buffer), base == 16 ? "%lX" : "%ld", v);
    print((const char *)buffer);
  }
};
static SerialClass Serial;

void setup();
void loop();

#endif
