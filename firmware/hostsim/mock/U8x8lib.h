#ifndef MOCK_U8X8LIB_H
#define MOCK_U8X8LIB_H
#include "hostsim.h"
#define U8X8_PIN_NONE 255
struct U8X8_SSD1306_128X32_UNIVISION_HW_I2C {
  explicit U8X8_SSD1306_128X32_UNIVISION_HW_I2C(uint8_t) {}
  bool begin() { return true; }
  void setBusClock(uint32_t) {}
  void setContrast(uint8_t value) { hs_display_contrast(value); }
  void setFlipMode(uint8_t on) { hs_display_flip(on); }
  void setPowerSave(uint8_t off) { hs_display_power(!off); }
  uint8_t drawTile(uint8_t x, uint8_t y, uint8_t count, uint8_t *tile) {
    for (uint8_t i = 0; i < count; i++) hs_display_tile(x + i, y, tile + 8 * i);
    return 1;
  }
};
#endif
