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
//   ROWS <32|64>               display height; with 64 only the top half is used (the lower half is blanked)
//   LOGO <offset> <32 hex>     16 bitmap bytes at byte offset (multiple of 16)
//   RAW <hex bytes>            raw controller command bytes (max 24)
//   FONTDATA <offset> <32 hex> 16 bytes of a font (772 bytes, U8x8 format) into RAM
//   FONTTEXT <row> <text>      text with the uploaded font, row 0..3, 16 characters

#include <Arduino.h>
#include <U8x8lib.h>
#include <Wire.h>

static const uint8_t DISPLAY_ADDRESS = 0x3C;
static const uint16_t BITMAP_BYTES = 512;  // 128 x 32 / 8

U8X8_SSD1306_128X32_UNIVISION_HW_I2C display(U8X8_PIN_NONE);

// A font uploaded by trustctl, for trying it out before it is flashed into the
// device parameter block (see tools/build/gen_params.py). 49 chunks of 16 bytes.
static uint8_t fontRam[784];

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
    reply(F("TrustIssue-Calibration 0.3 display 128x32 or 128x64 (ROWS)"));
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
  } else if (!strcmp_P(text, PSTR("ROWS"))) {
    int rows = atoi(args);
    if (rows != 32 && rows != 64) {
      reply(F("ERR badarg"));
      return;
    }
    // multiplex ratio and COM pin configuration of the panel; the same bytes sit in the device parameter block
    uint8_t height[] = {0xA8, (uint8_t)(rows - 1), 0xDA, (uint8_t)(rows == 64 ? 0x12 : 0x02)};
    sendRaw(height, 4);
    if (rows == 64) {
      static const uint8_t blank[8] = {0, 0, 0, 0, 0, 0, 0, 0};
      for (uint8_t row = 4; row < 8; row++)
        for (uint8_t column = 0; column < 16; column++) display.drawTile(column, row, 1, (uint8_t *)blank);
    }
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("POWER"))) {
    display.setPowerSave(atoi(args) ? 0 : 1);
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
  } else if (!strcmp_P(text, PSTR("FONTDATA"))) {
    char *data = strchr(args, ' ');
    long offset = atol(args);
    if (!data || offset < 0 || offset + 16 > (long)sizeof(fontRam) || (offset & 15)) {
      reply(F("ERR badarg"));
      return;
    }
    data++;
    if (strlen(data) != 32 || !parseHexBytes(data, fontRam + offset, 16)) {
      reply(F("ERR badhex"));
      return;
    }
    reply(F("OK"));
  } else if (!strcmp_P(text, PSTR("FONTTEXT"))) {
    int row = atoi(args);
    char *message = strchr(args, ' ');
    if (!message || row < 0 || row > 3 || fontRam[0] != 32 || fontRam[1] != 127 ||
        fontRam[2] != 1 || fontRam[3] != 1) {
      reply(F("ERR nofont"));
      return;
    }
    message++;
    uint8_t length = strlen(message);
    for (uint8_t column = 0; column < 16; column++) {
      char c = column < length ? message[column] : ' ';
      uint8_t glyph = (c >= 32 && c < 128) ? c - 32 : '?' - 32;
      display.drawTile(column, row, 1, fontRam + 4 + glyph * 8);
    }
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
  Serial.println(F("TrustIssue-Calibration 0.3 display 128x32 or 128x64 (ROWS)"));
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
