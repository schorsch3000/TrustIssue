#include "tc_protocol.h"

#include <string.h>

#include "tc_base32.h"

// Returns the next space separated field and moves the cursor behind it.
// An empty field (two spaces in a row) is returned as an empty string.
static char *nextField(char **cursor) {
  char *start = *cursor;
  if (!start) return NULL;
  char *space = strchr(start, ' ');
  if (space) {
    *space = '\0';
    *cursor = space + 1;
  } else {
    *cursor = NULL;
  }
  return start;
}

static bool parseNumber(const char *text, uint32_t max, uint32_t *out) {
  if (!text || !*text) return false;
  uint32_t value = 0;
  for (; *text; text++) {
    if (*text < '0' || *text > '9') return false;
    uint64_t next = (uint64_t)value * 10 + (*text - '0');
    if (next > max) return false;
    value = next;
  }
  *out = value;
  return true;
}

static bool parseSigned(const char *text, int32_t min, int32_t max, int32_t *out) {
  bool negative = text && *text == '-';
  uint32_t magnitude;
  if (!parseNumber(negative ? text + 1 : text, 1000000, &magnitude)) return false;
  int32_t value = negative ? -(int32_t)magnitude : (int32_t)magnitude;
  if (value < min || value > max) return false;
  *out = value;
  return true;
}

static int8_t hexDigit(char c) {
  if (c >= '0' && c <= '9') return c - '0';
  if (c >= 'a' && c <= 'f') return c - 'a' + 10;
  if (c >= 'A' && c <= 'F') return c - 'A' + 10;
  return -1;
}

TcParseResult tc_parse_command(char *line, TcCommand *command) {
  char *cursor = line;
  char *word = nextField(&cursor);
  memset(command, 0, sizeof(*command));

  if (!strcmp(word, "HELLO") || !strcmp(word, "LIST") || !strcmp(word, "GETTZ") ||
      !strcmp(word, "GETCFG") || !strcmp(word, "WIPE")) {
    if (cursor) return TC_PARSE_BADARG;  // these take no arguments
    command->type = !strcmp(word, "HELLO")    ? TC_CMD_HELLO
                    : !strcmp(word, "LIST")   ? TC_CMD_LIST
                    : !strcmp(word, "GETTZ")  ? TC_CMD_GETTZ
                    : !strcmp(word, "GETCFG") ? TC_CMD_GETCFG
                                              : TC_CMD_WIPE;
    return TC_PARSE_OK;
  }

  if (!strcmp(word, "DEL") || !strcmp(word, "TOKEN")) {
    bool token = word[0] == 'T';
    uint32_t id;
    if (!parseNumber(nextField(&cursor), TC_SLOT_COUNT - 1, &id)) return TC_PARSE_BADARG;
    command->id = id;
    if (token) {
      if (!parseNumber(nextField(&cursor), 0xFFFFFFFFUL, &command->time))
        return TC_PARSE_BADARG;
    }
    if (cursor) return TC_PARSE_BADARG;
    command->type = token ? TC_CMD_TOKEN : TC_CMD_DEL;
    return TC_PARSE_OK;
  }

  if (!strcmp(word, "SETTZ")) {
    int32_t quarters;
    if (!parseSigned(nextField(&cursor), -48, 56, &quarters) || cursor)
      return TC_PARSE_BADARG;
    command->type = TC_CMD_SETTZ;
    command->timezone = quarters;
    return TC_PARSE_OK;
  }

  if (!strcmp(word, "SETCFG")) {
    char *hex = nextField(&cursor);
    if (!hex || strlen(hex) != 2 || cursor) return TC_PARSE_BADARG;
    int8_t high = hexDigit(hex[0]), low = hexDigit(hex[1]);
    if (high < 0 || low < 0) return TC_PARSE_BADARG;
    uint8_t flags = (high << 4) | low;
    if (flags & ~TC_CONFIG_MASK) return TC_PARSE_BADARG;
    command->type = TC_CMD_SETCFG;
    command->config = flags;
    return TC_PARSE_OK;
  }

  if (!strcmp(word, "ADD")) {
    // ADD <base32> <algo> <digits> <period> <name...>
    char *secret = nextField(&cursor);
    char *algo = nextField(&cursor);
    uint32_t digits, period;
    if (!secret || !algo || strcmp(algo, "SHA1")) return TC_PARSE_BADARG;
    if (!parseNumber(nextField(&cursor), 8, &digits) || digits < 6)
      return TC_PARSE_BADARG;
    if (!parseNumber(nextField(&cursor), 60, &period) || (period != 30 && period != 60))
      return TC_PARSE_BADARG;
    if (!cursor || !*cursor || strlen(cursor) > TC_NAME_MAX) return TC_PARSE_BADARG;
    int16_t size = tc_base32_decode(secret, command->key.secret, TC_SECRET_BYTES);
    if (size < 0) return TC_PARSE_BADARG;
    command->key.algo = TC_ALGO_SHA1;
    command->key.digits = digits;
    command->key.period = period;
    strcpy(command->key.name, cursor);  // length checked above
    command->type = TC_CMD_ADD;
    return TC_PARSE_OK;
  }

  return TC_PARSE_UNKNOWN;
}
