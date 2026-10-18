#ifndef MOCK_WIRE_H
#define MOCK_WIRE_H
#include "hostsim.h"
// The only thing core.ino sends by hand is the SSD1306 invert command (0xA6 normal, 0xA7 inverted).
struct TwoWire {
  uint8_t first = 0xFF, second = 0xFF, count = 0;
  void beginTransmission(uint8_t) { count = 0; }
  void write(uint8_t b) {
    if (count == 0) first = b;
    if (count == 1) second = b;
    count++;
  }
  uint8_t endTransmission() {
    if (count == 2 && first == 0x00 && (second == 0xA6 || second == 0xA7)) hs_display_invert(second == 0xA7);
    return 0;
  }
};
static TwoWire Wire;
#endif
