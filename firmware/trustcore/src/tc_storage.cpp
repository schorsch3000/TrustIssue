#include "tc_storage.h"

static const uint16_t HEADER_BYTES = 4;
static const uint16_t SLOT_BYTES = 51;
static const uint8_t FLAGS_AT = 0;
static const uint8_t SECRET_AT = 1;
static const uint8_t NAME_AT = 21;
static const uint8_t CRC_AT = 49;
static const uint8_t CRC_COVERS = 49;

static const uint8_t HEADER_MAGIC = 0xA0;
static const uint8_t HEADER_VERSION = 1;

static const TcEeprom *eeprom;
static uint8_t defaultConfig;

void tc_store_begin(const TcEeprom *ops, uint8_t defaults) {
  eeprom = ops;
  defaultConfig = defaults;
}

uint16_t tc_crc16(const uint8_t *data, size_t size, uint16_t crc) {
  while (size--) {
    crc ^= (uint16_t)*data++ << 8;
    for (uint8_t bit = 0; bit < 8; bit++) {
      crc = (crc & 0x8000) ? (crc << 1) ^ 0x1021 : crc << 1;
    }
  }
  return crc;
}

uint8_t tc_crc8(const uint8_t *data, size_t size) {
  uint8_t crc = 0;
  while (size--) {
    crc ^= *data++;
    for (uint8_t bit = 0; bit < 8; bit++) {
      crc = (crc & 0x80) ? (crc << 1) ^ 0x07 : crc << 1;
    }
  }
  return crc;
}

static uint16_t slotAddress(uint8_t id) { return HEADER_BYTES + id * SLOT_BYTES; }

static void writeZeros(uint16_t address, uint8_t count) {
  while (count--) eeprom->write(address++, 0);
}

// ---------------------------------------------------------------- names

bool tc_name_valid(const char *name) {
  uint8_t length = 0;
  char previous = 0;
  for (; *name; name++, length++) {
    char c = *name;
    if (length >= TC_NAME_MAX) return false;
    bool allowed = (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
                   (c >= '0' && c <= '9') || c == '.' || c == ':' || c == '-' ||
                   c == ' ';
    if (!allowed) return false;
    if (c == ' ' && (length == 0 || previous == ' ')) return false;
    previous = c;
  }
  return length > 0 && previous != ' ';
}

static char lower(char c) { return (c >= 'A' && c <= 'Z') ? c + ('a' - 'A') : c; }

bool tc_name_equal(const char *a, const char *b) {
  while (*a && *b) {
    if (lower(*a++) != lower(*b++)) return false;
  }
  return *a == *b;
}

// ---------------------------------------------------------------- slots

bool tc_store_get(uint8_t id, TcKey *key) {
  if (id >= TC_SLOT_COUNT) return false;
  uint16_t base = slotAddress(id);
  uint8_t flags = eeprom->read(base + FLAGS_AT);
  if (!(flags & 0x01)) return false;
  uint16_t crc = 0xFFFF;
  for (uint8_t i = 0; i < CRC_COVERS; i++) {
    uint8_t value = eeprom->read(base + i);
    crc = tc_crc16(&value, 1, crc);
  }
  uint16_t stored = ((uint16_t)eeprom->read(base + CRC_AT) << 8) |
                    eeprom->read(base + CRC_AT + 1);
  if (crc != stored) return false;
  uint8_t algo = (flags >> 1) & 0x03;
  uint8_t digits = 6 + ((flags >> 3) & 0x03);
  if (algo != TC_ALGO_SHA1 || digits > 8) return false;
  if (key) {
    key->algo = algo;
    key->digits = digits;
    key->period = (flags & 0x20) ? 60 : 30;
    for (uint8_t i = 0; i < TC_SECRET_BYTES; i++)
      key->secret[i] = eeprom->read(base + SECRET_AT + i);
    for (uint8_t i = 0; i < TC_NAME_BYTES; i++)
      key->name[i] = eeprom->read(base + NAME_AT + i);
    key->name[TC_NAME_BYTES - 1] = '\0';
  }
  return true;
}

uint8_t tc_store_count() {
  uint8_t count = 0;
  for (uint8_t id = 0; id < TC_SLOT_COUNT; id++) count += tc_store_get(id, NULL);
  return count;
}

TcStatus tc_store_add(const TcKey *key, uint8_t *id) {
  if (key->algo != TC_ALGO_SHA1 || key->digits < 6 || key->digits > 8 ||
      (key->period != 30 && key->period != 60) || !tc_name_valid(key->name))
    return TC_BADARG;

  int8_t freeSlot = -1;
  for (uint8_t slot = 0; slot < TC_SLOT_COUNT; slot++) {
    TcKey existing;
    if (tc_store_get(slot, &existing)) {
      if (tc_name_equal(existing.name, key->name)) return TC_DUPLICATE;
    } else if (freeSlot < 0) {
      freeSlot = slot;
    }
  }
  if (freeSlot < 0) return TC_FULL;

  uint8_t image[SLOT_BYTES];
  image[FLAGS_AT] = 0x01 | (key->algo << 1) | ((key->digits - 6) << 3) |
                    (key->period == 60 ? 0x20 : 0);
  for (uint8_t i = 0; i < TC_SECRET_BYTES; i++) image[SECRET_AT + i] = key->secret[i];
  uint8_t nameLength = 0;
  while (key->name[nameLength]) nameLength++;
  for (uint8_t i = 0; i < TC_NAME_BYTES; i++)
    image[NAME_AT + i] = i < nameLength ? key->name[i] : 0;
  uint16_t crc = tc_crc16(image, CRC_COVERS, 0xFFFF);
  image[CRC_AT] = crc >> 8;
  image[CRC_AT + 1] = crc;

  // The CRC goes last: until then the slot is invalid, so power loss in the
  // middle never produces a half-written key that looks valid.
  uint16_t base = slotAddress(freeSlot);
  for (uint8_t i = 0; i < SLOT_BYTES; i++) eeprom->write(base + i, image[i]);
  if (id) *id = freeSlot;
  return TC_OK;
}

TcStatus tc_store_delete(uint8_t id) {
  if (!tc_store_get(id, NULL)) return TC_NOT_FOUND;
  uint16_t base = slotAddress(id);
  eeprom->write(base + FLAGS_AT, 0);  // invalid from here on
  writeZeros(base + SECRET_AT, TC_SECRET_BYTES);
  writeZeros(base + NAME_AT, TC_NAME_BYTES);
  writeZeros(base + CRC_AT, 2);
  return TC_OK;
}

void tc_store_wipe(TcProgress progress, void *context) {
  uint32_t used = 0;
  uint8_t count = 0;
  for (uint8_t id = 0; id < TC_SLOT_COUNT; id++) {
    if (tc_store_get(id, NULL)) {
      used |= (uint32_t)1 << id;
      count++;
    }
  }
  uint16_t total = (uint16_t)count * (TC_SECRET_BYTES + TC_NAME_BYTES + 3);
  uint16_t done = 0;
  // secrets first, byte 0 of every key, then byte 1 of every key, ...
  // (the first overwritten byte already invalidates a key through its CRC)
  for (uint8_t offset = 0; offset < TC_SECRET_BYTES; offset++) {
    for (uint8_t id = 0; id < TC_SLOT_COUNT; id++) {
      if (!(used & ((uint32_t)1 << id))) continue;
      eeprom->write(slotAddress(id) + SECRET_AT + offset, 0);
      if (progress) progress(++done, total, context);
    }
  }
  for (uint8_t offset = 0; offset < TC_NAME_BYTES; offset++) {
    for (uint8_t id = 0; id < TC_SLOT_COUNT; id++) {
      if (!(used & ((uint32_t)1 << id))) continue;
      eeprom->write(slotAddress(id) + NAME_AT + offset, 0);
      if (progress) progress(++done, total, context);
    }
  }
  for (uint8_t id = 0; id < TC_SLOT_COUNT; id++) {
    if (!(used & ((uint32_t)1 << id))) continue;
    writeZeros(slotAddress(id) + CRC_AT, 2);
    eeprom->write(slotAddress(id) + FLAGS_AT, 0);
    done += 3;
    if (progress) progress(done, total, context);
  }
}

// ---------------------------------------------------------------- header

static bool readHeader(uint8_t header[HEADER_BYTES]) {
  for (uint8_t i = 0; i < HEADER_BYTES; i++) header[i] = eeprom->read(i);
  return (header[0] & 0xF0) == HEADER_MAGIC && (header[0] & 0x0F) == HEADER_VERSION &&
         header[3] == tc_crc8(header, 3);
}

static void writeHeader(uint8_t timezone, uint8_t config) {
  uint8_t header[HEADER_BYTES] = {(uint8_t)(HEADER_MAGIC | HEADER_VERSION),
                                  timezone, config, 0};
  header[3] = tc_crc8(header, 3);
  for (uint8_t i = 0; i < HEADER_BYTES; i++) eeprom->write(i, header[i]);
}

int8_t tc_store_timezone() {
  uint8_t header[HEADER_BYTES];
  return readHeader(header) ? (int8_t)header[1] : 0;
}

uint8_t tc_store_config() {
  uint8_t header[HEADER_BYTES];
  return readHeader(header) ? header[2] : defaultConfig;
}

void tc_store_set_timezone(int8_t quarterHours) {
  uint8_t header[HEADER_BYTES];
  uint8_t config = readHeader(header) ? header[2] : defaultConfig;
  writeHeader((uint8_t)quarterHours, config);
}

void tc_store_set_config(uint8_t flags) {
  uint8_t header[HEADER_BYTES];
  uint8_t timezone = readHeader(header) ? header[1] : 0;
  writeHeader(timezone, flags);
}
