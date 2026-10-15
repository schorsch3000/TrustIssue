// Host tests for trustcore: g++ ... test_core.cpp ../trustcore/src/*.cpp
#include <stdio.h>
#include <string.h>

#include "../trustcore/src/tc_base32.h"
#include "../trustcore/src/tc_sha1.h"
#include "../trustcore/src/tc_totp.h"

static int failures;

static void check(bool ok, const char *what) {
  if (!ok) {
    failures++;
    printf("FAIL  %s\n", what);
  }
}

static void hex(const uint8_t *data, size_t size, char *out) {
  for (size_t i = 0; i < size; i++) sprintf(out + 2 * i, "%02x", data[i]);
}

static void checkSha1(const char *message, const char *expected) {
  TcSha1 ctx;
  uint8_t digest[20];
  char text[41];
  tc_sha1_init(ctx);
  tc_sha1_update(ctx, (const uint8_t *)message, strlen(message));
  tc_sha1_final(ctx, digest);
  hex(digest, 20, text);
  check(!strcmp(text, expected), message);
}

static void checkHmac(const uint8_t *key, size_t keySize, const char *message,
                      const char *expected, const char *what) {
  uint8_t digest[20];
  char text[41];
  tc_hmac_sha1(key, keySize, (const uint8_t *)message, strlen(message), digest);
  hex(digest, 20, text);
  check(!strcmp(text, expected), what);
}

int main() {
  // SHA-1 (FIPS 180 examples)
  checkSha1("", "da39a3ee5e6b4b0d3255bfef95601890afd80709");
  checkSha1("abc", "a9993e364706816aba3e25717850c26c9cd0d89d");
  checkSha1("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq",
            "84983e441c3bd26ebaae4aa1f95129e5e54670f1");

  // HMAC-SHA1 (RFC 2202 cases 1, 2 and 6: key longer than the block size)
  uint8_t key1[20];
  memset(key1, 0x0b, sizeof(key1));
  checkHmac(key1, 20, "Hi There", "b617318655057264e28bc0b6fb378c8ef146be00",
            "hmac case 1");
  checkHmac((const uint8_t *)"Jefe", 4, "what do ya want for nothing?",
            "effcdf6ae5eb2fa2d27416d5f184df9c259a7c79", "hmac case 2");
  uint8_t key6[80];
  memset(key6, 0xaa, sizeof(key6));
  checkHmac(key6, 80, "Test Using Larger Than Block-Size Key - Hash Key First",
            "aa4ae5e15272d00e95705637ce8a3b55ed402112", "hmac case 6");

  // TOTP (RFC 6238 appendix B, SHA-1, 8 digits, 30 s)
  const uint8_t *secret = (const uint8_t *)"12345678901234567890";
  struct {
    uint64_t time;
    const char *code;
  } vectors[] = {{59, "94287082"},         {1111111109, "07081804"},
                 {1111111111, "14050471"}, {1234567890, "89005924"},
                 {2000000000, "69279037"}, {20000000000ULL, "65353130"}};
  for (auto &v : vectors) {
    char text[16];
    tc_format_code(tc_hotp(secret, 20, v.time / 30, 8), 8, text);
    check(!strcmp(text, v.code), v.code);
  }
  char six[8];
  tc_format_code(tc_totp(secret, 20, 59, 30, 6), 6, six);
  check(!strcmp(six, "287082"), "6 digit code at t=59");

  // Base32 (RFC 4648 section 10)
  uint8_t out[32];
  int16_t n = tc_base32_decode("MZXW6YTBOI======", out, sizeof(out));
  check(n == 6 && !memcmp(out, "foobar", 6), "base32 foobar");
  n = tc_base32_decode("mzxw6ytboi", out, sizeof(out));
  check(n == 6 && !memcmp(out, "foobar", 6), "base32 lowercase, no padding");
  n = tc_base32_decode("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", out, sizeof(out));
  check(n == 20 && !memcmp(out, secret, 20), "base32 of the RFC 6238 secret");
  check(tc_base32_decode("MZXW 6YTB", out, sizeof(out)) == -1, "reject space");
  check(tc_base32_decode("MZXW1YTB", out, sizeof(out)) == -1, "reject digit 1");
  check(tc_base32_decode("MZ=XW", out, sizeof(out)) == -1, "reject data after =");
  check(tc_base32_decode("", out, sizeof(out)) == -1, "reject empty");
  check(tc_base32_decode("MZXW6YTBOI", out, 3) == -1, "reject too long");

  printf(failures ? "%d FAILED\n" : "all tests passed\n", failures);
  return failures != 0;
}
