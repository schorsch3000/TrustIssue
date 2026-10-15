#include "tc_base32.h"

static int8_t symbolValue(char c) {
  if (c >= 'A' && c <= 'Z') return c - 'A';
  if (c >= 'a' && c <= 'z') return c - 'a';
  if (c >= '2' && c <= '7') return c - '2' + 26;
  return -1;
}

int16_t tc_base32_decode(const char *text, uint8_t *out, size_t outCapacity) {
  uint16_t buffer = 0;
  uint8_t bits = 0;
  size_t count = 0;
  bool padding = false;
  for (; *text; text++) {
    if (*text == '=') {
      padding = true;
      continue;
    }
    if (padding) return -1;  // data after padding
    int8_t value = symbolValue(*text);
    if (value < 0) return -1;
    buffer = (buffer << 5) | value;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      if (count >= outCapacity) return -1;
      out[count++] = (uint8_t)(buffer >> bits);
    }
  }
  return count ? (int16_t)count : -1;
}
