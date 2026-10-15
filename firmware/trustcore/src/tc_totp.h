#ifndef TC_TOTP_H
#define TC_TOTP_H

#include <stddef.h>
#include <stdint.h>

// HOTP (RFC 4226) with HMAC-SHA1. digits is 1..9.
uint32_t tc_hotp(const uint8_t *secret, size_t secretSize, uint64_t counter,
                 uint8_t digits);

// TOTP (RFC 6238): counter = unixTime / period.
uint32_t tc_totp(const uint8_t *secret, size_t secretSize, uint32_t unixTime,
                 uint32_t period, uint8_t digits);

// Writes digits characters (leading zeros kept) plus a terminating NUL.
void tc_format_code(uint32_t code, uint8_t digits, char *out);

#endif
