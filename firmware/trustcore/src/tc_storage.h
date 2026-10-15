#ifndef TC_STORAGE_H
#define TC_STORAGE_H

#include <stddef.h>
#include <stdint.h>

// EEPROM layout (1024 bytes):
//   header, 4 bytes: [0] magic (high nibble 0xA) and layout version (low nibble)
//                    [1] timezone offset in 15 minute units (int8)
//                    [2] configuration flags
//                    [3] CRC-8 over bytes 0..2
//   20 slots of 51 bytes:
//     +0   flags: bit0 used, bits1-2 algorithm, bits3-4 digits-6, bit5 period 60 s
//     +1   secret, 20 bytes, zero padded (a shorter HMAC key padded with zeros
//          gives the same MAC, so no length field is needed)
//     +21  name, 28 bytes, NUL padded
//     +49  CRC-16 over bytes +0..+48
//   A slot counts as used only if the used bit is set and the CRC matches. The
//   CRC is written last, so a slot that is half written is simply unused.

#define TC_SLOT_COUNT 20
#define TC_SECRET_BYTES 20
#define TC_NAME_MAX 27
#define TC_NAME_BYTES 28

#define TC_ALGO_SHA1 0

#define TC_CFG_ROTATE 0x01
#define TC_CFG_INVERT 0x02
#define TC_CFG_LOGO 0x04
#define TC_CFG_AUTOWIPE 0x08

struct TcEeprom {
  uint8_t (*read)(uint16_t address);
  void (*write)(uint16_t address, uint8_t value);  // may skip unchanged bytes
};

struct TcKey {
  uint8_t algo;
  uint8_t digits;  // 6, 7 or 8
  uint8_t period;  // 30 or 60
  uint8_t secret[TC_SECRET_BYTES];
  char name[TC_NAME_BYTES];
};

enum TcStatus : uint8_t {
  TC_OK = 0,
  TC_BADARG,
  TC_DUPLICATE,
  TC_FULL,
  TC_NOT_FOUND,
};

// defaultConfig is used while the header is blank or damaged.
void tc_store_begin(const TcEeprom *eeprom, uint8_t defaultConfig);

// Names: 1..27 characters from A-Z a-z 0-9 . : - and space, no leading or
// trailing space, no double spaces.
bool tc_name_valid(const char *name);
// Case-insensitive comparison, as used for the uniqueness rule.
bool tc_name_equal(const char *a, const char *b);

// Reads a slot. Returns false if it is unused or corrupt. key may be NULL.
bool tc_store_get(uint8_t id, TcKey *key);
uint8_t tc_store_count();
TcStatus tc_store_add(const TcKey *key, uint8_t *id);
// Zeroes the whole slot, secret included.
TcStatus tc_store_delete(uint8_t id);

// Overwrites all keys, interleaved: byte 0 of every key, then byte 1 of every
// key, and so on, so that an interrupted wipe damages all keys evenly. The
// header (timezone, configuration) is kept. progress may be NULL.
typedef void (*TcProgress)(uint16_t done, uint16_t total, void *context);
void tc_store_wipe(TcProgress progress, void *context);

// Header fields. A blank or damaged header yields the defaults.
int8_t tc_store_timezone();
void tc_store_set_timezone(int8_t quarterHours);
uint8_t tc_store_config();
void tc_store_set_config(uint8_t flags);

uint16_t tc_crc16(const uint8_t *data, size_t size, uint16_t crc);
uint8_t tc_crc8(const uint8_t *data, size_t size);

#endif
