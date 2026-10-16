// Host tests for the EEPROM storage layer.
#include <stdio.h>
#include <string.h>

#include "../trustcore/src/tc_storage.h"

static uint8_t memory[1024];
static long writes;        // calls to write()
static long writeBudget;   // simulated power loss: writes beyond this are lost
static int failures;

static uint8_t ramRead(uint16_t address) { return memory[address]; }
static void ramWrite(uint16_t address, uint8_t value) {
  writes++;
  if (writeBudget >= 0 && writes > writeBudget) return;
  memory[address] = value;
}
static const TcEeprom RAM = {ramRead, ramWrite};

static void check(bool ok, const char *what) {
  if (!ok) {
    failures++;
    printf("FAIL  %s\n", what);
  }
}

static void blank() {
  memset(memory, 0xFF, sizeof(memory));
  writes = 0;
  writeBudget = -1;
  tc_store_begin(&RAM, TC_CFG_ROTATE);
}

static TcKey makeKey(const char *name, uint8_t fill = 0x11) {
  TcKey key;
  memset(&key, 0, sizeof(key));
  key.algo = TC_ALGO_SHA1;
  key.digits = 6;
  key.period = 30;
  memset(key.secret, fill, TC_SECRET_BYTES);
  strncpy(key.name, name, TC_NAME_BYTES - 1);
  return key;
}

static void testCrc() {
  const uint8_t text[] = "123456789";
  check(tc_crc16(text, 9, 0xFFFF) == 0x29B1, "crc16 check value");
  check(tc_crc8(text, 9) == 0xF4, "crc8 check value");
}

static void testNames() {
  check(tc_name_valid("GitHub"), "plain name");
  check(tc_name_valid("a"), "one char");
  check(tc_name_valid("work: aws-root.prod 2"), "all allowed symbols");
  check(tc_name_valid("a@b.c_d/e (x)+y=\"'`~!#$%^"), "punctuation, part 1");
  check(tc_name_valid("&*[]{}<>?,;|\\ ~"), "punctuation, part 2");
  check(tc_name_valid("user@example.com"), "an e-mail address");
  check(tc_name_valid("123456789012345678901234567"), "27 chars");
  check(!tc_name_valid("1234567890123456789012345678"), "28 chars");
  check(!tc_name_valid(""), "empty");
  check(!tc_name_valid(" GitHub"), "leading space");
  check(!tc_name_valid("GitHub "), "trailing space");
  check(!tc_name_valid("Git  Hub"), "double space");
  check(tc_name_valid("Git_Hub"), "underscore");
  check(!tc_name_valid("Git\x7F" "Hub"), "DEL");
  check(!tc_name_valid("Git\x01Hub"), "control character");
  check(!tc_name_valid("Git\t" "Hub"), "tab");
  check(!tc_name_valid("Git\xFF" "Hub"), "byte above 0x7F");
  check(!tc_name_valid("G\xC3\xBCtHub"), "umlaut (UTF-8)");
  check(!tc_name_valid("Git\nHub"), "newline");
  check(tc_name_equal("GitHub", "github"), "case-insensitive equal");
  check(!tc_name_equal("GitHub", "GitHub2"), "different");
  check(!tc_name_equal("Git", "GitHub"), "prefix is not equal");
}

static void testAddGetDelete() {
  blank();
  check(tc_store_count() == 0, "blank EEPROM has no keys");
  uint8_t id = 99;
  TcKey key = makeKey("GitHub", 0x42);
  key.digits = 8;
  key.period = 60;
  check(tc_store_add(&key, &id) == TC_OK && id == 0, "add first key");
  TcKey back;
  check(tc_store_get(0, &back), "get first key");
  check(!strcmp(back.name, "GitHub") && back.digits == 8 && back.period == 60 &&
            !memcmp(back.secret, key.secret, TC_SECRET_BYTES),
        "key round trip");
  TcKey second = makeKey("AWS", 0x43);
  check(tc_store_add(&second, &id) == TC_OK && id == 1, "second key gets slot 1");
  TcKey duplicate = makeKey("github", 0x44);
  check(tc_store_add(&duplicate, &id) == TC_DUPLICATE, "case-insensitive duplicate");
  TcKey bad = makeKey("bad\x01name");
  check(tc_store_add(&bad, &id) == TC_BADARG, "bad name");
  bad = makeKey("ok");
  bad.digits = 9;
  check(tc_store_add(&bad, &id) == TC_BADARG, "bad digits");
  bad = makeKey("ok");
  bad.period = 45;
  check(tc_store_add(&bad, &id) == TC_BADARG, "bad period");
  check(tc_store_count() == 2, "two keys");

  check(tc_store_delete(0) == TC_OK, "delete slot 0");
  bool zero = true;
  for (int i = 4; i < 4 + 51; i++) zero &= memory[i] == 0;
  check(zero, "deleted slot is all zeros (secret gone)");
  check(tc_store_delete(0) == TC_NOT_FOUND, "delete twice");
  check(tc_store_delete(30) == TC_NOT_FOUND, "delete out of range");
  TcKey third = makeKey("Third", 0x45);
  check(tc_store_add(&third, &id) == TC_OK && id == 0, "freed slot is reused");
}

static void testFull() {
  blank();
  for (int i = 0; i < TC_SLOT_COUNT; i++) {
    char name[16];
    snprintf(name, sizeof(name), "key %d", i);
    TcKey key = makeKey(name);
    uint8_t id;
    check(tc_store_add(&key, &id) == TC_OK && id == i, "fill slot");
  }
  TcKey extra = makeKey("one too many");
  uint8_t id;
  check(tc_store_add(&extra, &id) == TC_FULL, "store full");
  check(tc_store_count() == TC_SLOT_COUNT, "all slots used");
}

static void testCorruption() {
  blank();
  TcKey key = makeKey("GitHub");
  uint8_t id;
  tc_store_add(&key, &id);
  memory[4 + 5] ^= 0x01;  // flip a secret bit
  check(!tc_store_get(0, NULL), "corrupt slot is unused");
  blank();
  tc_store_add(&key, &id);
  memory[4 + 22] ^= 0x01;  // flip a name bit
  check(!tc_store_get(0, NULL), "corrupt name makes the slot unused");
}

static void testInterruptedAdd() {
  // power loss after every possible number of writes: the slot must either be
  // fully valid or unused, never valid with wrong content
  for (long budget = 0; budget < 51; budget++) {
    blank();
    TcKey key = makeKey("GitHub", 0x77);
    writeBudget = budget;
    uint8_t id;
    tc_store_add(&key, &id);
    writeBudget = -1;
    TcKey back;
    if (tc_store_get(0, &back)) {
      check(!strcmp(back.name, "GitHub") && back.secret[0] == 0x77,
            "valid slot after interrupted add has correct content");
    }
  }
  blank();
  TcKey key = makeKey("GitHub", 0x77);
  writeBudget = 49;  // everything except the last CRC byte
  uint8_t id;
  tc_store_add(&key, &id);
  writeBudget = -1;
  check(!tc_store_get(0, NULL), "slot without its final CRC byte is unused");
}

static void testHeader() {
  blank();
  check(tc_store_timezone() == 0, "default timezone");
  check(tc_store_config() == TC_CFG_ROTATE, "default config on blank EEPROM");
  tc_store_set_timezone(8);  // UTC+2
  check(tc_store_timezone() == 8, "timezone stored");
  check(tc_store_config() == TC_CFG_ROTATE, "config keeps its default");
  tc_store_set_config(TC_CFG_INVERT | TC_CFG_LOGO);
  check(tc_store_config() == (TC_CFG_INVERT | TC_CFG_LOGO), "config stored");
  check(tc_store_timezone() == 8, "timezone survives config change");
  tc_store_set_timezone(-20);
  check(tc_store_timezone() == -20, "negative timezone");
  memory[1] ^= 0x10;  // damage the header
  check(tc_store_timezone() == 0 && tc_store_config() == TC_CFG_ROTATE,
        "damaged header falls back to defaults");
  TcKey key = makeKey("still fine");
  uint8_t id;
  check(tc_store_add(&key, &id) == TC_OK, "keys do not depend on the header");
}

static void testWipe() {
  blank();
  tc_store_set_timezone(4);
  tc_store_set_config(TC_CFG_LOGO);
  for (int i = 0; i < TC_SLOT_COUNT; i++) {
    char name[16];
    snprintf(name, sizeof(name), "key %d", i);
    TcKey key = makeKey(name, 0x55);
    uint8_t id;
    tc_store_add(&key, &id);
  }
  uint8_t before[1024];
  memcpy(before, memory, sizeof(before));
  tc_store_wipe(NULL, NULL);
  check(tc_store_count() == 0, "no keys after wipe");
  bool zero = true;
  for (int i = 4; i < 1024; i++) zero &= memory[i] == 0;
  check(zero, "slot area is all zeros after wipe");
  check(!memcmp(memory, before, 4), "wipe keeps the header");
  check(tc_store_timezone() == 4 && tc_store_config() == TC_CFG_LOGO,
        "timezone and config survive the wipe");

  // interrupted wipe: after any number of writes every key lost almost the same
  // number of secret bytes (secrets are all non-zero, so every write counts)
  for (long budget = 0; budget <= 20 * 20; budget += 7) {
    memcpy(memory, before, sizeof(memory));
    writes = 0;
    writeBudget = budget;
    tc_store_wipe(NULL, NULL);
    writeBudget = -1;
    int low = 99, high = 0;
    for (int id = 0; id < TC_SLOT_COUNT; id++) {
      int zeroed = 0;
      for (int b = 0; b < TC_SECRET_BYTES; b++) zeroed += memory[4 + id * 51 + 1 + b] == 0;
      if (zeroed < low) low = zeroed;
      if (zeroed > high) high = zeroed;
    }
    check(high - low <= 1, "interrupted wipe damages all keys evenly");
    if (budget >= 20) check(tc_store_count() == 0, "every key invalid after one pass");
  }

  // progress callback
  memcpy(memory, before, sizeof(memory));
  struct Progress {
    uint16_t last, total;
  } progress = {0, 0};
  tc_store_wipe(
      [](uint16_t done, uint16_t total, void *context) {
        Progress *p = (Progress *)context;
        p->last = done;
        p->total = total;
      },
      &progress);
  check(progress.last == progress.total && progress.total == 20 * (20 + 28 + 3),
        "progress reaches the total");

  // wiping an empty store does nothing
  blank();
  tc_store_wipe(NULL, NULL);
  check(writes == 0, "wipe of an empty store writes nothing");
}

int main() {
  testCrc();
  testNames();
  testAddGetDelete();
  testFull();
  testCorruption();
  testInterruptedAdd();
  testHeader();
  testWipe();
  printf(failures ? "%d FAILED\n" : "all storage tests passed\n", failures);
  return failures != 0;
}
