#include "tc_totp.h"

#include "tc_sha1.h"

static const uint32_t POWERS_OF_TEN[] = {1UL,      10UL,      100UL,
                                         1000UL,   10000UL,   100000UL,
                                         1000000UL, 10000000UL, 100000000UL,
                                         1000000000UL};

uint32_t tc_hotp(const uint8_t *secret, size_t secretSize, uint64_t counter,
                 uint8_t digits) {
  uint8_t message[8];
  for (int8_t i = 7; i >= 0; i--) {
    message[i] = (uint8_t)counter;
    counter >>= 8;
  }
  uint8_t hash[20];
  tc_hmac_sha1(secret, secretSize, message, sizeof(message), hash);
  uint8_t offset = hash[19] & 0x0F;
  uint32_t binary = ((uint32_t)(hash[offset] & 0x7F) << 24) |
                    ((uint32_t)hash[offset + 1] << 16) |
                    ((uint32_t)hash[offset + 2] << 8) | hash[offset + 3];
  return binary % POWERS_OF_TEN[digits];
}

uint32_t tc_totp(const uint8_t *secret, size_t secretSize, uint32_t unixTime,
                 uint32_t period, uint8_t digits) {
  return tc_hotp(secret, secretSize, unixTime / period, digits);
}

void tc_format_code(uint32_t code, uint8_t digits, char *out) {
  for (int8_t i = digits - 1; i >= 0; i--) {
    out[i] = '0' + code % 10;
    code /= 10;
  }
  out[digits] = '\0';
}
