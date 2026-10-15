#include <Wire.h>

void setup() {
  Serial.begin(115200);
  Wire.begin();
  delay(500);
  Serial.println(F("I2C scan start"));
}

void loop() {
  byte found = 0;
  for (byte addr = 1; addr < 127; addr++) {
    Wire.beginTransmission(addr);
    if (Wire.endTransmission() == 0) {
      Serial.print(F("found 0x"));
      Serial.println(addr, HEX);
      found++;
    }
  }
  if (!found) Serial.println(F("nothing found"));
  Serial.println(F("--"));
  delay(3000);
}
