// Runs the RFC test vectors for trustcore on the real chip and times a TOTP.

#include <Arduino.h>
#include <tc_base32.h>
#include <tc_sha1.h>
#include <tc_totp.h>

static const uint8_t SECRET[] = "12345678901234567890";

static bool checkTotp(uint32_t time, const char *expected) {
  char text[10];
  tc_format_code(tc_hotp(SECRET, 20, time / 30, 8), 8, text);
  bool ok = !strcmp(text, expected);
  Serial.print(F("totp t="));
  Serial.print(time);
  Serial.print(' ');
  Serial.print(text);
  Serial.println(ok ? F(" PASS") : F(" FAIL"));
  return ok;
}

void setup() {
  Serial.begin(9600);
  delay(1500);
  bool ok = true;
  ok &= checkTotp(59, "94287082");
  ok &= checkTotp(1111111109, "07081804");
  ok &= checkTotp(1111111111, "14050471");
  ok &= checkTotp(1234567890, "89005924");
  ok &= checkTotp(2000000000, "69279037");

  uint8_t secret[20];
  int16_t n = tc_base32_decode("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", secret, sizeof(secret));
  bool base32Ok = n == 20 && !memcmp(secret, SECRET, 20);
  Serial.println(base32Ok ? F("base32 PASS") : F("base32 FAIL"));
  ok &= base32Ok;

  uint32_t start = micros();
  uint32_t code = tc_totp(SECRET, 20, 1234567890, 30, 6);
  uint32_t elapsed = micros() - start;
  Serial.print(F("one TOTP takes "));
  Serial.print(elapsed);
  Serial.print(F(" us, code "));
  Serial.println(code);
  Serial.println(ok ? F("ALL PASS") : F("SOME FAILED"));
}

void loop() {}
