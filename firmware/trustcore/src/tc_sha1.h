#ifndef TC_SHA1_H
#define TC_SHA1_H

#include <stddef.h>
#include <stdint.h>

struct TcSha1 {
  uint32_t state[5];
  uint32_t length;  // bytes hashed so far
  uint8_t block[64];
  uint8_t used;
};

void tc_sha1_init(TcSha1 &ctx);
void tc_sha1_update(TcSha1 &ctx, const uint8_t *data, size_t size);
void tc_sha1_final(TcSha1 &ctx, uint8_t digest[20]);

// HMAC-SHA1 (RFC 2104). Keys longer than 64 bytes are hashed first.
void tc_hmac_sha1(const uint8_t *key, size_t keySize, const uint8_t *message,
                  size_t messageSize, uint8_t digest[20]);

#endif
