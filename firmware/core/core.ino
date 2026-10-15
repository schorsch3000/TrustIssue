// Trust Issues core firmware: TOTP vault with serial protocol, display and button.
//
// Build with firmware/core/build.sh (it enlarges the serial receive buffer).
// Not implemented yet: idle logo, autonomous wipe, flash-patched device
// parameters (contrast, font, logo); those use compile-time constants for now.

#include <Arduino.h>
#include <avr/boot.h>
#include <EEPROM.h>
#include <U8x8lib.h>
#include <Wire.h>
#include <tc_protocol.h>
#include <tc_storage.h>
#include <tc_time.h>
#include <tc_totp.h>

#include "core_types.h"
#include "params_default.h"

// ---------------------------------------------------------------- settings

static const char VERSION[] PROGMEM = "0.1.0";
static const uint8_t BUTTON_PIN = 2;
static const uint8_t DISPLAY_ADDRESS = 0x3C;
static const uint8_t DEFAULT_CONFIG = 0;

static const uint16_t DEBOUNCE_MS = 25;
static const uint16_t ACCEPT_MIN_MS = 1000;
static const uint16_t ACCEPT_MAX_MS = 2000;
static const uint16_t WIPE_MIN_MS = 5000;
static const uint16_t WIPE_MAX_MS = 6000;
static const uint32_t REQUEST_TIMEOUT_MS = 30000;
static const uint32_t BOOT_LOGO_MS = 5000;    // after power-up the logo shows this long (with the logo option)
static const uint32_t IDLE_WAIT_MS = 10000;   // the idle screen stays this long
static const uint32_t LOGO_FULL_MS = 30000;   // then the logo at full contrast ...
static const uint32_t LOGO_DIM_MS = 120000;   // ... then dimmed, then the display turns off
// A finished device has the lock bits of tools/user/final_lock.sh (0xCC) and the fuses the
// design wants: BOOTRST set, 2 KB boot section, EESAVE unprogrammed (high fuse bits 3..0 = 1010).
// Anything else is a development unit, and the idle screen and HELLO say so.
static const uint8_t FINAL_LOCK_BITS = 0xCC;
static bool deviceUnlocked;

static void checkLock() {
  uint8_t lock = boot_lock_fuse_bits_get(GET_LOCK_BITS);
  uint8_t high = boot_lock_fuse_bits_get(GET_HIGH_FUSE_BITS);
  deviceUnlocked = lock != FINAL_LOCK_BITS || (high & 0x0F) != 0x0A;
}

static const uint16_t RESULT_SHOW_MS = 1200;
static const uint16_t TOKEN_SHOW_MS = 5000;  // "Done" and the code, so it can be read off the device

// ---------------------------------------------------------------- device parameters

// The block PARAMS (params_default.h) sits at flash address 0x7000 and is
// patched by `trustctl flash`: contrast values, the font and the idle logo.
static const uint16_t PARAM_FLAGS = 5;  // bit0 font valid, bit1 logo valid
static const uint16_t PARAM_CONTRAST_FULL = 6;
static const uint16_t PARAM_CONTRAST_DIM = 7;
static const uint16_t PARAM_FONT = 0x10;
static const uint16_t PARAM_LOGO = 0x314;

static uint8_t param(uint16_t offset) { return pgm_read_byte(&PARAMS[offset]); }
static bool logoAvailable() { return param(PARAM_FLAGS) & 0x02; }

// ---------------------------------------------------------------- EEPROM

static uint8_t eepromRead(uint16_t address) { return EEPROM.read(address); }
static void eepromWrite(uint16_t address, uint8_t value) { EEPROM.update(address, value); }
static const TcEeprom EEPROM_OPS = {eepromRead, eepromWrite};

// ---------------------------------------------------------------- display

U8X8_SSD1306_128X32_UNIVISION_HW_I2C oled(U8X8_PIN_NONE);

static char grid[4][16];   // what the screen shows, 4 rows of 16 characters (bit 7: inverted)
static uint8_t dirtyRows;  // rows whose tiles must be redrawn
static uint8_t fillPixels; // progress bar: this many columns from the left are inverted
static bool displayOn = true;
static uint32_t lastActivity;

// Where the accept window starts on the progress bar, in pixel columns: the
// bar is solid (inverted) before it and a checkerboard inside it.
static uint8_t windowStart = 128;

static void drawTile(uint8_t column, uint8_t row) {
  uint8_t tile[8];
  char c = grid[row][column];
  bool highlight = c & 0x80;  // bit 7 of a grid character: shown inverted
  c &= 0x7F;
  uint8_t glyph = c >= 32 ? c - 32 : '?' - 32;
  for (uint8_t i = 0; i < 8; i++) {
    uint8_t x = column * 8 + i;
    tile[i] = param(PARAM_FONT + 4 + glyph * 8 + i);
    if (highlight) tile[i] = ~tile[i];
    if (x < fillPixels) {
      if (x < windowStart) {
        tile[i] = ~tile[i];  // before the window: the bar is solid
      } else {
        tile[i] ^= (x & 2) ? 0xCC : 0x33;  // inside the window: a checkerboard of 2x2 pixel squares
      }
    }
  }
  oled.drawTile(column, row, 1, tile);
}

static void render() {
  if (!displayOn) return;
  for (uint8_t row = 0; row < 4; row++) {
    if (!(dirtyRows & (1 << row))) continue;
    for (uint8_t column = 0; column < 16; column++) drawTile(column, row);
    dirtyRows &= ~(1 << row);
  }
}

static void setRow(uint8_t row, const char *text, uint8_t maxChars = 16) {
  uint8_t i = 0;
  for (; i < 16 && i < maxChars && text[i]; i++) grid[row][i] = text[i];
  for (; i < 16; i++) grid[row][i] = ' ';
  dirtyRows |= 1 << row;
}

static void setRowP(uint8_t row, PGM_P text) {
  char buffer[17];
  strncpy_P(buffer, text, 16);
  buffer[16] = '\0';
  setRow(row, buffer);
}

static void clearScreen() {
  for (uint8_t row = 0; row < 4; row++) setRow(row, "");
  fillPixels = 0;
}

// Name over rows 1 and 2, first 16 characters on the first row.
static void showName(const char *name) {
  setRow(1, name, 16);
  setRow(2, strlen(name) > 16 ? name + 16 : "");
}

static void setFill(uint8_t pixels) {
  if (pixels > 128) pixels = 128;
  if (pixels == fillPixels) return;
  render();
  uint8_t low = min(pixels, fillPixels) / 8, high = min(max(pixels, fillPixels) / 8, 15);
  fillPixels = pixels;
  if (!displayOn) return;
  for (uint8_t row = 0; row < 4; row++)
    for (uint8_t column = low; column <= high; column++) drawTile(column, row);
}

static void sendDisplayCommand(uint8_t command) {
  Wire.beginTransmission(DISPLAY_ADDRESS);
  Wire.write((uint8_t)0x00);
  Wire.write(command);
  Wire.endTransmission();
}

static void applyConfig(uint8_t config) {
  oled.setFlipMode(config & TC_CFG_ROTATE ? 1 : 0);
  sendDisplayCommand(config & TC_CFG_INVERT ? 0xA7 : 0xA6);
}

enum IdlePhase : uint8_t { IDLE_SCREEN, IDLE_BOOT_LOGO, IDLE_LOGO_FULL, IDLE_LOGO_DIM, IDLE_OFF };
static IdlePhase idlePhase = IDLE_SCREEN;

static void wakeDisplay() {
  lastActivity = millis();
  if (idlePhase == IDLE_SCREEN && displayOn) return;
  idlePhase = IDLE_SCREEN;
  oled.setContrast(param(PARAM_CONTRAST_FULL));
  if (!displayOn) oled.setPowerSave(0);
  displayOn = true;
  dirtyRows = 0x0F;  // the screen shows the text grid again, not the logo
  render();
}

static void sleepDisplay() {
  oled.setPowerSave(1);
  displayOn = false;
  idlePhase = IDLE_OFF;
}

// The 512 byte logo of the parameter block, drawn tile by tile.
static void showLogo(uint8_t contrast) {
  uint8_t tile[8];
  for (uint8_t index = 0; index < 64; index++) {
    for (uint8_t i = 0; i < 8; i++) tile[i] = param(PARAM_LOGO + index * 8 + i);
    oled.drawTile(index % 16, index / 16, 1, tile);
  }
  oled.setContrast(contrast);
}

static void showIdle() {
  clearScreen();
  setRowP(1, PSTR("Trust Issues"));
  char count[17];
  snprintf_P(count, sizeof(count), PSTR("%u keys"), tc_store_count());
  setRow(2, count);
  if (deviceUnlocked) {  // inverted, so it cannot pass for a normal idle screen
    setRowP(3, PSTR(" NOT LOCKED     "));
    for (uint8_t column = 0; column < 16; column++) grid[3][column] |= 0x80;
  }
  render();
}

// ---------------------------------------------------------------- button

static bool buttonStableDown, buttonLastRaw;
static uint32_t buttonLastChange, buttonPressedAt;

// Debounced: a change counts after the level was stable for DEBOUNCE_MS, and
// its time is that of the last raw edge. A release reports the hold time.
static ButtonEvent pollButton(uint32_t *held) {
  bool raw = !digitalRead(BUTTON_PIN);
  uint32_t now = millis();
  if (raw != buttonLastRaw) {
    buttonLastRaw = raw;
    buttonLastChange = now;
  } else if (raw != buttonStableDown && now - buttonLastChange >= DEBOUNCE_MS) {
    buttonStableDown = raw;
    if (raw) {
      buttonPressedAt = buttonLastChange;
      return BUTTON_PRESSED;
    }
    *held = buttonLastChange - buttonPressedAt;
    return BUTTON_RELEASED;
  }
  return BUTTON_NONE;
}

static uint32_t buttonHeldMs() { return buttonStableDown ? millis() - buttonPressedAt : 0; }

// ---------------------------------------------------------------- requests

static State state = STATE_IDLE;
static TcCommand pending;
static uint32_t pendingStart;
static uint32_t pendingMinMs, pendingMaxMs;
// True if the button was already down when the request came: that press does not
// count, the user has to let go and press again.
static bool pressIgnored;

static uint32_t resultUntil;
static uint32_t lastClockCheck;
static uint8_t shownMinute = 255;

static uint32_t unixNow() { return pending.time + (millis() - pendingStart) / 1000; }

static void showTokenTime() {
  int8_t zone = tc_store_timezone();
  TcDateTime now = tc_local_time(unixNow(), zone);
  char text[20];
  tc_format_local(now, text);
  setRow(3, text);
  shownMinute = now.minute;
}

// "TOKEN" on the left of the first row, the timezone offset ("UTC+2") on the right.
static void showTokenHeader() {
  char text[17];
  memset(text, ' ', 16);
  memcpy(text, "TOKEN", 5);
  char offset[12];
  tc_format_offset(tc_store_timezone(), true, offset);
  uint8_t length = strlen(offset);
  memcpy(text + 16 - length, offset, length);
  text[16] = '\0';
  setRow(0, text);
}

// A row with two settings ("Rot:off Inv:on"); those that this request changes are inverted.
static void settingsRow(uint8_t row, PGM_P labelA, uint8_t bitA, PGM_P labelB, uint8_t bitB) {
  PGM_P labels[2] = {labelA, labelB};
  uint8_t bits[2] = {bitA, bitB};
  uint8_t changed = pending.config ^ tc_store_config();
  uint8_t start[2], end[2], position = 0;
  char text[20];
  for (uint8_t i = 0; i < 2; i++) {
    if (i) text[position++] = ' ';
    start[i] = position;
    strcpy_P(text + position, labels[i]);
    position += strlen(text + position);
    strcpy_P(text + position, pending.config & bits[i] ? PSTR("on") : PSTR("off"));
    position += strlen(text + position);
    end[i] = position;
  }
  setRow(row, text);
  for (uint8_t i = 0; i < 2; i++) {
    if (!(changed & bits[i])) continue;
    for (uint8_t column = start[i]; column < end[i] && column < 16; column++) grid[row][column] |= 0x80;
  }
}

static void showPending() {
  clearScreen();
  char text[20];
  switch (pending.type) {
    case TC_CMD_TOKEN:
      showTokenHeader();
      showName(pending.key.name);
      showTokenTime();
      break;
    case TC_CMD_ADD:
      setRowP(0, PSTR("ADD KEY"));
      showName(pending.key.name);
      snprintf_P(text, sizeof(text), PSTR("%u digits %u s"), pending.key.digits,
                 pending.key.period);
      setRow(3, text);
      break;
    case TC_CMD_DEL:
      setRowP(0, PSTR("DELETE KEY"));
      showName(pending.key.name);
      setRowP(3, PSTR("Hold 1-2 s"));
      break;
    case TC_CMD_SETTZ:
      setRowP(0, PSTR("SET TIMEZONE"));
      tc_format_offset(pending.timezone, true, text);
      setRow(1, text);
      setRowP(3, PSTR("Hold 1-2 s"));
      break;
    case TC_CMD_SETCFG:
      setRowP(0, PSTR("SETTINGS"));
      settingsRow(1, PSTR("Rot:"), TC_CFG_ROTATE, PSTR("Inv:"), TC_CFG_INVERT);
      settingsRow(2, PSTR("Logo:"), TC_CFG_LOGO, PSTR("Wp:"), TC_CFG_AUTOWIPE);
      setRowP(3, PSTR("Hold 1-2 s"));
      break;
    case TC_CMD_WIPE:
      setRowP(0, PSTR("WIPE ALL KEYS?"));
      setRowP(1, PSTR("Hold 5-6 s"));
      snprintf_P(text, sizeof(text), PSTR("%u keys"), tc_store_count());
      setRow(3, text);
      break;
    default:
      break;
  }
  wakeDisplay();
  render();
}

// Doubles each of the 4 bits of a nibble: bit k becomes bits 2k and 2k+1.
static uint8_t doubleBits(uint8_t nibble) {
  uint8_t out = 0;
  for (uint8_t k = 0; k < 4; k++) {
    if (nibble & (1 << k)) out |= 3 << (2 * k);
  }
  return out;
}

// One tile of a short word in large letters (the 8x8 font doubled to 16x16): half 0 is the upper
// and half 1 the lower tile row of the word. Computed tile by tile so that the whole screen can be
// replaced in one pass: from the progress bar straight to the word, with no blank screen or
// flicker in between.
static void bigTile(uint8_t column, uint8_t half, const char *text, uint8_t start, uint8_t length, uint8_t *tile) {
  memset(tile, 0, 8);
  if (column < start || column >= start + 2 * length) return;
  char c = text[(column - start) / 2];
  uint8_t glyph = c >= 32 ? c - 32 : '?' - 32;
  uint8_t side = (column - start) % 2;
  for (uint8_t i = 0; i < 8; i++) {
    uint8_t bits = param(PARAM_FONT + 4 + glyph * 8 + side * 4 + i / 2);  // every source column becomes two
    tile[i] = doubleBits(half == 0 ? bits & 0x0F : bits >> 4);
  }
}

// Large letters (at most 8 characters per line) replace whatever is on the display, for showMs.
// One word is centered on tile rows 1 and 2; with a second line the words fill rows 0-1 and 2-3.
static void showResult(const char *text, const char *second = NULL, uint16_t showMs = RESULT_SHOW_MS) {
  clearScreen();  // the text grid is blank now, nothing is drawn yet
  wakeDisplay();  // only draws if the display was off or showed the logo
  uint8_t length = strlen(text);
  uint8_t start = (16 - 2 * length) / 2;
  uint8_t secondLength = second ? strlen(second) : 0;
  uint8_t secondStart = (16 - 2 * secondLength) / 2;
  uint8_t tile[8];
  for (uint8_t row = 0; row < 4; row++) {
    for (uint8_t column = 0; column < 16; column++) {
      if (second) {
        if (row < 2) bigTile(column, row, text, start, length, tile);
        else bigTile(column, row - 2, second, secondStart, secondLength, tile);
      } else if (row == 1 || row == 2) {
        bigTile(column, row - 1, text, start, length, tile);
      } else {
        memset(tile, 0, 8);
      }
      oled.drawTile(column, row, 1, tile);
    }
  }
  dirtyRows = 0;
  resultUntil = millis() + showMs;
}

static const __FlashStringHelper *statusText(TcStatus status) {
  switch (status) {
    case TC_BADARG: return F("ERR BADARG");
    case TC_DUPLICATE: return F("ERR DUPLICATE");
    case TC_FULL: return F("ERR FULL");
    case TC_NOT_FOUND: return F("ERR NOTFOUND");
    default: return F("OK");
  }
}

static TcStatus precheckAdd(const TcKey &key) {
  if (!tc_name_valid(key.name)) return TC_BADARG;
  bool freeSlot = false;
  for (uint8_t id = 0; id < TC_SLOT_COUNT; id++) {
    TcKey existing;
    if (tc_store_get(id, &existing)) {
      if (tc_name_equal(existing.name, key.name)) return TC_DUPLICATE;
    } else {
      freeSlot = true;
    }
  }
  return freeSlot ? TC_OK : TC_FULL;
}

static void wipeProgress(uint16_t done, uint16_t total, void *) {
  setFill((uint32_t)done * 128 / total);
}

static void finishPending(Outcome outcome) {
  if (outcome != OUTCOME_ACCEPTED) {
    if (pending.type == TC_CMD_SETCFG) applyConfig(tc_store_config());  // undo the preview
    bool timedOut = outcome == OUTCOME_TIMEOUT;
    Serial.println(timedOut ? F("ERR TIMEOUT") : F("ERR DENIED"));  // too long counts as denied
    showResult(timedOut ? "Timeout" : outcome == OUTCOME_TOO_LONG ? "Too long" : "Denied");
  } else {
    char shownCode[10] = "";
    switch (pending.type) {
      case TC_CMD_TOKEN: {
        TcKey key;
        if (!tc_store_get(pending.id, &key)) {
          Serial.println(F("ERR NOTFOUND"));
          break;
        }
        char code[10];
        tc_format_code(tc_totp(key.secret, TC_SECRET_BYTES, unixNow(), key.period, key.digits),
                       key.digits, code);
        memset(key.secret, 0, sizeof(key.secret));
        Serial.print(F("OK "));
        Serial.println(code);
        strcpy(shownCode, code);
        break;
      }
      case TC_CMD_ADD: {
        uint8_t id = 0;
        TcStatus status = tc_store_add(&pending.key, &id);
        if (status == TC_OK) {
          Serial.print(F("OK "));
          Serial.println(id);
        } else {
          Serial.println(statusText(status));
        }
        break;
      }
      case TC_CMD_DEL:
        Serial.println(statusText(tc_store_delete(pending.id)));
        break;
      case TC_CMD_SETTZ:
        tc_store_set_timezone(pending.timezone);
        Serial.println(F("OK"));
        break;
      case TC_CMD_SETCFG:
        tc_store_set_config(pending.config);
        applyConfig(pending.config);
        Serial.println(F("OK"));
        break;
      case TC_CMD_WIPE:
        windowStart = 128;  // the wipe's own progress is a plain bar
        setFill(0);         // starting from the empty bar
        tc_store_wipe(wipeProgress, NULL);
        Serial.println(F("OK"));
        break;
      default:
        break;
    }
    if (shownCode[0]) showResult("Done", shownCode, TOKEN_SHOW_MS);
    else showResult("Done");
    memset(shownCode, 0, sizeof(shownCode));
  }
  memset(&pending, 0, sizeof(pending));  // no secret stays in RAM
  state = STATE_IDLE;
}

static void startPending() {
  pendingStart = millis();
  bool wipe = pending.type == TC_CMD_WIPE;
  pendingMinMs = wipe ? WIPE_MIN_MS : ACCEPT_MIN_MS;
  pendingMaxMs = wipe ? WIPE_MAX_MS : ACCEPT_MAX_MS;
  windowStart = pendingMinMs * 128UL / pendingMaxMs;  // the bar spans 0 to the end of the window
  state = STATE_PENDING;
  pressIgnored = buttonStableDown;
  Serial.println(F("PENDING"));
  showPending();
  if (pending.type == TC_CMD_SETCFG) applyConfig(pending.config);  // preview
}

static void printBanner() {
  Serial.print(F("TrustIssue "));
  Serial.print((const __FlashStringHelper *)VERSION);
  Serial.print(F(" proto 1"));
#ifdef AUTONOMOUS_WIPE
  Serial.print(F(" caps=autowipe"));
#endif
  if (deviceUnlocked) Serial.print(F(" unlocked"));
  Serial.println();
}

static void handleCommand() {
  switch (pending.type) {
    case TC_CMD_HELLO:
      printBanner();
      return;
    case TC_CMD_LIST:
      for (uint8_t id = 0; id < TC_SLOT_COUNT; id++) {
        TcKey key;
        if (!tc_store_get(id, &key)) continue;
        Serial.print(F("KEY "));
        Serial.print(id);
        Serial.print(' ');
        Serial.println(key.name);
      }
      Serial.println(F("END"));
      return;
    case TC_CMD_GETTZ:
      Serial.print(F("OK "));
      Serial.println(tc_store_timezone());
      return;
    case TC_CMD_GETCFG: {
      uint8_t config = tc_store_config();
      Serial.print(F("OK "));
      if (config < 16) Serial.print('0');
      Serial.println(config, HEX);
      return;
    }
    case TC_CMD_ADD: {
      TcStatus status = precheckAdd(pending.key);
      if (status != TC_OK) {
        Serial.println(statusText(status));
        return;
      }
      break;
    }
    case TC_CMD_DEL:
    case TC_CMD_TOKEN:
      if (!tc_store_get(pending.id, &pending.key)) {
        Serial.println(F("ERR NOTFOUND"));
        return;
      }
      break;
    case TC_CMD_SETCFG:
#ifndef AUTONOMOUS_WIPE
      if (pending.config & TC_CFG_AUTOWIPE) {  // not built into this firmware
        Serial.println(F("ERR BADARG"));
        return;
      }
#endif
      break;
    case TC_CMD_WIPE:
      if (tc_store_count() == 0) {  // nothing to destroy, nothing to confirm
        Serial.println(F("OK"));
        return;
      }
      break;
    default:
      break;
  }
  startPending();
}

// ---------------------------------------------------------------- serial

static char line[100];
static uint8_t lineLength;
static bool lineOverflow;

static void processLine() {
  if (state != STATE_IDLE) {
    Serial.println(F("ERR BUSY"));
    return;
  }
  TcParseResult result = tc_parse_command(line, &pending);
  if (result == TC_PARSE_UNKNOWN) {
    Serial.println(F("ERR UNKNOWN"));
  } else if (result == TC_PARSE_BADARG) {
    Serial.println(F("ERR BADARG"));
  } else {
    handleCommand();
  }
  if (state != STATE_PENDING) memset(&pending, 0, sizeof(pending));
}

static void readSerial() {
  while (Serial.available()) {
    char c = Serial.read();
    if (c == '\r') continue;
    if (c == '\n') {
      line[lineLength] = '\0';
      if (lineOverflow) {
        Serial.println(F("ERR BADARG"));
      } else if (lineLength) {
        processLine();
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

// ---------------------------------------------------------------- main loop

static void servicePending(ButtonEvent event, uint32_t held) {
  uint32_t now = millis();
  if (now - pendingStart > REQUEST_TIMEOUT_MS) {
    finishPending(OUTCOME_TIMEOUT);
    return;
  }
  if (pressIgnored) {
    if (event == BUTTON_RELEASED) pressIgnored = false;
    return;
  }
  if (buttonStableDown && buttonHeldMs() > pendingMaxMs) {  // past the window: no need to wait for the release
    finishPending(OUTCOME_TOO_LONG);
    return;
  }
  if (event == BUTTON_RELEASED) {
    bool accepted = held >= pendingMinMs && held <= pendingMaxMs;
    finishPending(accepted ? OUTCOME_ACCEPTED : OUTCOME_DENIED);
    return;
  }
  if (buttonStableDown) {
    setFill(min(buttonHeldMs(), pendingMaxMs) * 128UL / pendingMaxMs);
  } else if (pending.type == TC_CMD_TOKEN && now - lastClockCheck >= 250) {
    lastClockCheck = now;
    TcDateTime local = tc_local_time(unixNow(), tc_store_timezone());
    if (local.minute != shownMinute) {
      showTokenTime();
      render();
    }
  }
}

// ---------------------------------------------------------------- autonomous wipe
//
// Without any host (a power bank is enough): with the capability built in
// (AUTONOMOUS_WIPE, see build.sh) and switched on (config bit), hold the button
// at idle and release it between 10 and 11 s. Then a countdown bar drains (a
// press aborts), and the same gesture as the bootloader's follows: tap, tap,
// press again and hold 2 s. Then the keys are wiped. Any mistake starts the
// gesture over, 20 s without success cancel everything.

#ifdef AUTONOMOUS_WIPE
static const uint32_t AUTO_HOLD_MIN_MS = 10000;
static const uint32_t AUTO_HOLD_MAX_MS = 11000;
static const uint32_t AUTO_HOLD_SHOW_MS = 2000;  // the bar appears after this long
static const uint32_t AUTO_COUNTDOWN_MS = 4000;
static const uint32_t AUTO_TOTAL_MS = 20000;
static const uint32_t AUTO_TAP_MAX_MS = 600;
static const uint32_t AUTO_GAP_MAX_MS = 800;
static const uint32_t AUTO_HOLD_MS = 2000;

enum AutoPhase : uint8_t { AW_COUNTDOWN, AW_TAP, AW_HOLD_WAIT, AW_HOLD };
static AutoPhase autoPhase;
static uint8_t autoTaps;
static uint32_t autoStart, autoLastRelease;
static bool autoArmed, autoHoldBar;

static bool autoWipeEnabled() { return (tc_store_config() & TC_CFG_AUTOWIPE) && tc_store_count() > 0; }

static void autoGestureScreen() {
  clearScreen();
  setRowP(0, PSTR("WIPE ALL KEYS?"));
  setRowP(1, PSTR("TAP TAP HOLD"));
  setRowP(2, PSTR("2 SECONDS"));
  render();
}

static void startAutoWipe() {
  state = STATE_AUTOWIPE;
  autoPhase = AW_COUNTDOWN;
  autoTaps = 0;
  autoStart = millis();
  windowStart = 128;
  clearScreen();
  setRowP(0, PSTR("WIPE ALL KEYS?"));
  setRowP(1, PSTR("WAIT"));
  render();
  setFill(128);
}

// Moves the progress bar to a new length in quick steps of 4 columns instead of jumping.
static void sweepFill(uint8_t target) {
  while (fillPixels != target) {
    uint8_t distance = fillPixels < target ? target - fillPixels : fillPixels - target;
    if (distance > 4) distance = 4;
    setFill(fillPixels < target ? fillPixels + distance : fillPixels - distance);
  }
}

static void autoFinish(const char *text) {
  state = STATE_IDLE;
  showResult(text);
}

static void autoRestartGesture() {
  autoTaps = 0;
  autoPhase = AW_TAP;
  sweepFill(0);
}

static void serviceAutoWipe(ButtonEvent event, uint32_t held) {
  uint32_t now = millis();
  if (now - autoStart > AUTO_TOTAL_MS) {
    autoFinish("Cancel");
    return;
  }
  switch (autoPhase) {
    case AW_COUNTDOWN:
      if (buttonStableDown || event == BUTTON_PRESSED) {  // a press aborts
        autoFinish("Cancel");
      } else if (now - autoStart >= AUTO_COUNTDOWN_MS) {
        autoGestureScreen();
        autoRestartGesture();
      } else {
        setFill(128 - (now - autoStart) * 128UL / AUTO_COUNTDOWN_MS);
      }
      break;
    case AW_TAP:
      if (event == BUTTON_RELEASED) {
        if (held <= AUTO_TAP_MAX_MS) {
          autoTaps = 1;
          autoLastRelease = now;
          autoPhase = AW_HOLD_WAIT;
          sweepFill(43);
        } else {
          autoRestartGesture();
        }
      }
      break;
    case AW_HOLD_WAIT:  // the second tap, then the third press
      if (event == BUTTON_PRESSED && buttonPressedAt - autoLastRelease > AUTO_GAP_MAX_MS) {
        autoRestartGesture();
      } else if (event == BUTTON_PRESSED && autoTaps == 2) {
        autoPhase = AW_HOLD;
      } else if (event == BUTTON_RELEASED && autoTaps == 1 && held <= AUTO_TAP_MAX_MS) {
        autoTaps = 2;
        autoLastRelease = now;
        sweepFill(86);
      } else if (event == BUTTON_RELEASED) {
        autoRestartGesture();
      } else if (!buttonStableDown && now - autoLastRelease > AUTO_GAP_MAX_MS) {
        autoRestartGesture();
      }
      break;
    case AW_HOLD: {
      if (event == BUTTON_RELEASED) {
        autoRestartGesture();
        break;
      }
      uint32_t ms = buttonHeldMs();
      setFill(86 + min(ms, AUTO_HOLD_MS) * 42UL / AUTO_HOLD_MS);
      if (ms >= AUTO_HOLD_MS) {
        clearScreen();
        setRowP(1, PSTR("WIPING"));
        render();
        windowStart = 128;
        setFill(0);
        tc_store_wipe(wipeProgress, NULL);
        autoFinish("Wiped");
      }
      break;
    }
  }
}

// At idle: a long hold shows a bar to the end of the 10 to 11 s window.
static void autoIdleHold(ButtonEvent event, uint32_t held) {
  if (event == BUTTON_PRESSED) {
    autoArmed = autoWipeEnabled();
    autoHoldBar = false;
    return;
  }
  if (!autoArmed) return;
  if (event == BUTTON_RELEASED) {
    autoArmed = false;
    if (autoHoldBar) {
      autoHoldBar = false;
      if (held >= AUTO_HOLD_MIN_MS && held <= AUTO_HOLD_MAX_MS) {
        startAutoWipe();
      } else {
        showIdle();
      }
    }
    return;
  }
  if (!buttonStableDown) return;
  uint32_t ms = buttonHeldMs();
  lastActivity = millis();  // keep the display awake while the button is held
  if (autoHoldBar && ms > AUTO_HOLD_MAX_MS) {  // past the window: stop here, the release will be ignored
    autoArmed = autoHoldBar = false;
    showResult("Too long");
    return;
  }
  if (ms < AUTO_HOLD_SHOW_MS) return;
  if (!autoHoldBar) {
    autoHoldBar = true;
    windowStart = AUTO_HOLD_MIN_MS * 128UL / AUTO_HOLD_MAX_MS;
    clearScreen();
    setRowP(0, PSTR("WIPE ALL KEYS?"));
    setRowP(1, PSTR("HOLD 10 S"));
    render();
  }
  setFill(min(ms, AUTO_HOLD_MAX_MS) * 128UL / AUTO_HOLD_MAX_MS);
}
#endif

static void serviceIdle(ButtonEvent event, uint32_t held) {
  uint32_t now = millis();
  if (resultUntil && (int32_t)(now - resultUntil) >= 0) {
    resultUntil = 0;
    showIdle();
  }
  if (event == BUTTON_PRESSED) {
    bool wasIdleScreen = idlePhase == IDLE_SCREEN && displayOn;
    wakeDisplay();
    if (!wasIdleScreen) showIdle();  // a press that only wakes the display does nothing else
#ifdef AUTONOMOUS_WIPE
    autoIdleHold(event, held);
#endif
    return;
  }
#ifdef AUTONOMOUS_WIPE
  autoIdleHold(event, held);
  if (state == STATE_AUTOWIPE || autoHoldBar) return;  // the hold or the flow owns the display
#endif
  // idle sequence: the screen, then (with the logo option) the logo at full
  // contrast, the logo dimmed, and finally the display off
  // `now` was read before autoIdleHold() may have set lastActivity a few
  // milliseconds later; an unsigned difference would then wrap to about 49 days
  // and put the display to sleep at once.
  int32_t sinceActivity = (int32_t)(now - lastActivity);
  uint32_t quiet = sinceActivity < 0 ? 0 : (uint32_t)sinceActivity;
  bool logo = (tc_store_config() & TC_CFG_LOGO) && logoAvailable();
  if (idlePhase == IDLE_BOOT_LOGO) {  // the logo after power-up, then the idle screen
    if (quiet > BOOT_LOGO_MS) {
      idlePhase = IDLE_SCREEN;
      showIdle();
      lastActivity = now;
    }
  } else if (idlePhase == IDLE_SCREEN && quiet > IDLE_WAIT_MS) {
    if (logo) {
      idlePhase = IDLE_LOGO_FULL;
      showLogo(param(PARAM_CONTRAST_FULL));
    } else {
      sleepDisplay();
    }
  } else if (idlePhase == IDLE_LOGO_FULL && quiet > IDLE_WAIT_MS + LOGO_FULL_MS) {
    idlePhase = IDLE_LOGO_DIM;
    oled.setContrast(param(PARAM_CONTRAST_DIM));
  } else if (idlePhase == IDLE_LOGO_DIM && quiet > IDLE_WAIT_MS + LOGO_FULL_MS + LOGO_DIM_MS) {
    sleepDisplay();
  }
}

void setup() {
  Serial.begin(9600);
  pinMode(BUTTON_PIN, INPUT_PULLUP);
  buttonStableDown = buttonLastRaw = !digitalRead(BUTTON_PIN);
  checkLock();
  tc_store_begin(&EEPROM_OPS, DEFAULT_CONFIG);
  oled.begin();
  oled.setBusClock(400000);
  oled.setContrast(param(PARAM_CONTRAST_FULL));
  applyConfig(tc_store_config());
  showIdle();
  lastActivity = millis();
  if ((tc_store_config() & TC_CFG_LOGO) && logoAvailable()) {  // power-up: logo first
    idlePhase = IDLE_BOOT_LOGO;
    showLogo(param(PARAM_CONTRAST_FULL));
  }
  printBanner();
}

void loop() {
  readSerial();
  uint32_t held = 0;
  ButtonEvent event = pollButton(&held);
  if (state == STATE_PENDING) {
    servicePending(event, held);
#ifdef AUTONOMOUS_WIPE
  } else if (state == STATE_AUTOWIPE) {
    serviceAutoWipe(event, held);
#endif
  } else {
    serviceIdle(event, held);
  }
}
