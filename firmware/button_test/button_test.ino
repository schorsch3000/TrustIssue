// Button test for the Nano: debounce and hold-window classification.
//
// The debounce logic here is meant to move into the core firmware.
// Output (9600 baud): "PRESS" and "RELEASE held=<ms> <class> glitches=<n>".

#include <Arduino.h>

static const uint8_t BUTTON_PIN = 2;
static const uint16_t DEBOUNCE_MS = 25;
static const uint16_t ACCEPT_MIN_MS = 1000;
static const uint16_t ACCEPT_MAX_MS = 2000;
static const uint16_t WIPE_MIN_MS = 5000;
static const uint16_t WIPE_MAX_MS = 6000;

static bool stableDown;       // debounced state, true = pressed
static bool lastRawDown;
static uint32_t lastRawChange;
static uint32_t pressedAt;    // time of the (last) raw edge of the press
static uint16_t glitches;

static const __FlashStringHelper *classify(uint32_t held) {
  if (held >= WIPE_MIN_MS && held <= WIPE_MAX_MS) return F("WIPE");
  if (held < ACCEPT_MIN_MS) return F("DENY-SHORT");
  if (held <= ACCEPT_MAX_MS) return F("ACCEPT");
  return F("DENY-LONG");
}

void setup() {
  Serial.begin(9600);
  pinMode(BUTTON_PIN, INPUT_PULLUP);
  lastRawDown = stableDown = !digitalRead(BUTTON_PIN);
  Serial.println(F("TrustIssue-ButtonTest 0.1"));
}

void loop() {
  bool rawDown = !digitalRead(BUTTON_PIN);
  uint32_t now = millis();
  if (rawDown != lastRawDown) {
    lastRawDown = rawDown;
    lastRawChange = now;
    // a change that flips back before the debounce time is a glitch
    if (rawDown == stableDown) glitches++;
  } else if (rawDown != stableDown && now - lastRawChange >= DEBOUNCE_MS) {
    stableDown = rawDown;
    if (stableDown) {
      pressedAt = lastRawChange;
      Serial.println(F("PRESS"));
    } else {
      uint32_t held = lastRawChange - pressedAt;
      Serial.print(F("RELEASE held="));
      Serial.print(held);
      Serial.print(' ');
      Serial.print(classify(held));
      Serial.print(F(" glitches="));
      Serial.println(glitches);
      glitches = 0;
    }
  }
}
