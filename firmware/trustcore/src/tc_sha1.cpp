#include "tc_sha1.h"

static inline uint32_t rotateLeft(uint32_t value, uint8_t bits) {
  return (value << bits) | (value >> (32 - bits));
}

// The message schedule is kept in a 16 word circular buffer to save RAM.
static void processBlock(uint32_t state[5], const uint8_t block[64]) {
  uint32_t w[16];
  for (uint8_t i = 0; i < 16; i++) {
    w[i] = ((uint32_t)block[4 * i] << 24) | ((uint32_t)block[4 * i + 1] << 16) |
           ((uint32_t)block[4 * i + 2] << 8) | block[4 * i + 3];
  }
  uint32_t a = state[0], b = state[1], c = state[2], d = state[3], e = state[4];
  for (uint8_t i = 0; i < 80; i++) {
    if (i >= 16) {
      w[i & 15] = rotateLeft(
          w[(i - 3) & 15] ^ w[(i - 8) & 15] ^ w[(i - 14) & 15] ^ w[i & 15], 1);
    }
    uint32_t f, k;
    if (i < 20) {
      f = (b & c) | (~b & d);
      k = 0x5A827999UL;
    } else if (i < 40) {
      f = b ^ c ^ d;
      k = 0x6ED9EBA1UL;
    } else if (i < 60) {
      f = (b & c) | (b & d) | (c & d);
      k = 0x8F1BBCDCUL;
    } else {
      f = b ^ c ^ d;
      k = 0xCA62C1D6UL;
    }
    uint32_t next = rotateLeft(a, 5) + f + e + k + w[i & 15];
    e = d;
    d = c;
    c = rotateLeft(b, 30);
    b = a;
    a = next;
  }
  state[0] += a;
  state[1] += b;
  state[2] += c;
  state[3] += d;
  state[4] += e;
}

void tc_sha1_init(TcSha1 &ctx) {
  ctx.state[0] = 0x67452301UL;
  ctx.state[1] = 0xEFCDAB89UL;
  ctx.state[2] = 0x98BADCFEUL;
  ctx.state[3] = 0x10325476UL;
  ctx.state[4] = 0xC3D2E1F0UL;
  ctx.length = 0;
  ctx.used = 0;
}

void tc_sha1_update(TcSha1 &ctx, const uint8_t *data, size_t size) {
  ctx.length += size;
  while (size--) {
    ctx.block[ctx.used++] = *data++;
    if (ctx.used == 64) {
      processBlock(ctx.state, ctx.block);
      ctx.used = 0;
    }
  }
}

void tc_sha1_final(TcSha1 &ctx, uint8_t digest[20]) {
  uint32_t lengthBytes = ctx.length;
  uint8_t byte = 0x80;
  tc_sha1_update(ctx, &byte, 1);
  byte = 0;
  while (ctx.used != 56) tc_sha1_update(ctx, &byte, 1);
  // 64 bit big endian bit length (lengthBytes * 8); a 32 bit byte count means
  // the top 24 bits are always zero
  uint8_t bitLength[8] = {0,
                          0,
                          0,
                          (uint8_t)(lengthBytes >> 29),
                          (uint8_t)(lengthBytes >> 21),
                          (uint8_t)(lengthBytes >> 13),
                          (uint8_t)(lengthBytes >> 5),
                          (uint8_t)(lengthBytes << 3)};
  tc_sha1_update(ctx, bitLength, 8);
  for (uint8_t i = 0; i < 5; i++) {
    digest[4 * i] = ctx.state[i] >> 24;
    digest[4 * i + 1] = ctx.state[i] >> 16;
    digest[4 * i + 2] = ctx.state[i] >> 8;
    digest[4 * i + 3] = ctx.state[i];
  }
}

void tc_hmac_sha1(const uint8_t *key, size_t keySize, const uint8_t *message,
                  size_t messageSize, uint8_t digest[20]) {
  uint8_t shortKey[20];
  TcSha1 ctx;
  if (keySize > 64) {
    tc_sha1_init(ctx);
    tc_sha1_update(ctx, key, keySize);
    tc_sha1_final(ctx, shortKey);
    key = shortKey;
    keySize = 20;
  }
  uint8_t pad[64];
  uint8_t inner[20];
  for (uint8_t i = 0; i < 64; i++) pad[i] = (i < keySize ? key[i] : 0) ^ 0x36;
  tc_sha1_init(ctx);
  tc_sha1_update(ctx, pad, 64);
  tc_sha1_update(ctx, message, messageSize);
  tc_sha1_final(ctx, inner);
  for (uint8_t i = 0; i < 64; i++) pad[i] = (i < keySize ? key[i] : 0) ^ 0x5C;
  tc_sha1_init(ctx);
  tc_sha1_update(ctx, pad, 64);
  tc_sha1_update(ctx, inner, 20);
  tc_sha1_final(ctx, digest);
}
