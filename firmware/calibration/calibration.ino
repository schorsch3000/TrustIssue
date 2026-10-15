// Trust Issues calibration firmware.
//
// Lets the host try out the display (text, contrast, rotation, inversion,
// raw controller commands, logo bitmaps) without any secrets involved.
// It never touches the EEPROM.
//
// Line protocol, 9600 baud, every line is answered with "OK" or "ERR <why>":
//   HELLO
//   CLEAR
//   TEXT <row> <text>          8x8 text, row 0..3
//   BIG <row> <text>           2x2 text, uses rows <row> and <row>+1
//   CONTRAST <0..255>
//   FLIP <0|1>                 rotate 180 degrees
//   INVERT <0|1>
//   POWER <0|1>                display off / on
//   FONT <n>                   select candidate font n and draw a lookalike sample
//   LOGO <offset> <32 hex>     16 bitmap bytes at byte offset (multiple of 16)
//   RAW <hex bytes>            raw controller command bytes (max 24)

#include <Arduino.h>
#include <U8x8lib.h>
#include <Wire.h>

static const uint8_t DISPLAY_ADDRESS = 0x3C;
static const uint16_t BITMAP_BYTES = 512;  // 128 x 32 / 8

U8X8_SSD1306_128X32_UNIVISION_HW_I2C display(U8X8_PIN_NONE);

static const uint8_t *const fonts[] = {
    u8x8_font_chroma48medium8_r, u8x8_font_amstrad_cpc_extended_r,
    u8x8_font_artossans8_r,      u8x8_font_artosserif8_r,
    u8x8_font_pcsenior_r,        u8x8_font_pressstart2p_r,
    u8x8_font_pxplusibmcga_r,    u8x8_font_pxplusibmcgathin_r,
    u8x8_font_torussansbold8_r,  u8x8_font_victoriabold8_r,
    u8x8_font_victoriamedium8_r, u8x8_font_5x8_r,
};
static const uint8_t FONT_COUNT = sizeof(fonts) / sizeof(fonts[0]);

static char line[96];
static uint8_t lineLength;
static bool lineOverflow;

static int8_t hexValue(char c) {
  if (c >= '0' && c <= '9') return c - '0';
  if (c >= 'a' && c <= 'f') return c - 'a' + 10;
  if (c >= 'A' && c <= 'F') return c - 'A' + 10;
  return -1;
}

static bool parseHexBytes(const char *text, uint8_t *out, uint8_t count) {
  for (uint8_t i = 0; i < count; i++) {
    int8_t high = hexValue(text[2 * i]);
    int8_t low = hexValue(text[2 * i + 1]);
    if (high < 0 || low < 0) return false;
    out[i] = (high << 4) | low;
  }
  return true;
}

static void sendRaw(const uint8_t *bytes, uint8_t count) {
  Wire.beginTransmission(DISPLAY_ADDRESS);
  Wire.write((uint8_t)0x00);  // control byte: command stream
  for (uint8_t i = 0; i < count; i++) Wire.write(bytes[i]);
  Wire.endTransmission();
}

static void reply(const __FlashStringHelper *text) { Serial.println(text); }

static void handleLine(char *text) {
  char *args = strchr(text, ' ');
  if (args) {
    *args++ = '\0';
    while (*args == ' ') args++;
  } else {
    args = text + strlen(text);
  }

  if (!strcmp_P(text, PSTR("HELLO"))) {
    reply(F("TrustIssue-Calibration 0.1 display 128x32"));
  } else if (!strcmp_P(text, PSTR("CLEAR"))) {
    display.clear();
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("TEXT")) || !strcmp_P(text, PSTR("BIG"))) {
    bool big = text[0] == 'B';
    int row = atoi(args);
    char *message = strchr(args, ' ');
    if (!message || row < 0 || row > (big ? 2 : 3)) {
      reply(F("ERR badarg"));
      return;
    }
    message++;
    if (big) {
      display.clearLine(row);
      display.clearLine(row + 1);
      display.draw2x2String(0, row, message);
    } else {
      display.clearLine(row);
      display.drawString(0, row, message);
    }
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("CONTRAST"))) {
    int value = atoi(args);
    if (!*args || value < 0 || value > 255) {
      reply(F("ERR badarg"));
      return;
    }
    display.setContrast(value);
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("FLIP"))) {
    display.setFlipMode(atoi(args) ? 1 : 0);
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("INVERT"))) {
    uint8_t command = atoi(args) ? 0xA7 : 0xA6;
    sendRaw(&command, 1);
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("POWER"))) {
    display.setPowerSave(atoi(args) ? 0 : 1);
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("FONT"))) {
    int index = atoi(args);
    if (index < 0 || index >= FONT_COUNT) {
      reply(F("ERR badarg"));
      return;
    }
    display.setFont(fonts[index]);
    display.clear();
    display.drawString(0, 0, "Il1 O0oQ 5S 8B");
    display.drawString(0, 1, "GitHub paypa1");
    display.drawString(0, 2, "rn m vv cl .:-");
    display.setCursor(0, 3);
    display.print(F("font "));
    display.print(index);
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("LOGO"))) {
    char *data = strchr(args, ' ');
    long offset = atol(args);
    if (!data || offset < 0 || offset + 16 > BITMAP_BYTES || (offset & 15)) {
      reply(F("ERR badarg"));
      return;
    }
    data++;
    uint8_t chunk[16];
    if (strlen(data) != 32 || !parseHexBytes(data, chunk, 16)) {
      reply(F("ERR badhex"));
      return;
    }
    uint16_t tile = offset / 8;  // 16 tiles per row, a chunk is 2 tiles
    display.drawTile(tile % 16, tile / 16, 2, chunk);
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("RAW"))) {
    uint8_t length = strlen(args);
    uint8_t bytes[24];
    if (length == 0 || (length & 1) || length / 2 > sizeof(bytes) ||
        !parseHexBytes(args, bytes, length / 2)) {
      reply(F("ERR badhex"));
      return;
    }
    sendRaw(bytes, length / 2);
    reply(F("OK"));
  } else {
    reply(F("ERR unknown"));
  }
}

void setup() {
  Serial.begin(9600);
  display.begin();
  display.setFont(u8x8_font_chroma48medium8_r);
  display.drawString(0, 0, "Trust Issues");
  display.drawString(0, 1, "calibration");
  Serial.println(F("TrustIssue-Calibration 0.1 display 128x32"));
}

void loop() {
  while (Serial.available()) {
    char c = Serial.read();
    if (c == '\r') continue;
    if (c == '\n') {
      line[lineLength] = '\0';
      if (lineOverflow) {
        reply(F("ERR toolong"));
      } else if (lineLength) {
        handleLine(line);
      }
      lineLength = 0;
      lineOverflow = false;
    } else if (lineLength < sizeof(line) - 1) {
      line[lineLength++] = c;
    } else {
      lineOverflow = true;
    }
  }
}
