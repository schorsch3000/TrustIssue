#ifndef TC_BASE32_H
#define TC_BASE32_H

#include <stddef.h>
#include <stdint.h>

// Strict RFC 4648 Base32 decoder for TOTP secrets: A-Z and 2-7 (either case),
// optional '=' padding at the end, nothing else (no spaces or dashes).
// Returns the number of decoded bytes, or -1 for invalid input or if the
// result does not fit into outCapacity bytes.
int16_t tc_base32_decode(const char *text, uint8_t *out, size_t outCapacity);

#endif
