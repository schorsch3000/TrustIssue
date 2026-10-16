// Host tests for the protocol parser.
#include <stdio.h>
#include <string.h>

#include "../trustcore/src/tc_protocol.h"

static int failures;

static void check(bool ok, const char *what) {
  if (!ok) {
    failures++;
    printf("FAIL  %s\n", what);
  }
}

static TcParseResult parse(const char *text, TcCommand *command) {
  char line[128];
  strncpy(line, text, sizeof(line) - 1);
  line[sizeof(line) - 1] = '\0';
  return tc_parse_command(line, command);
}

static void expect(const char *text, TcParseResult result) {
  TcCommand command;
  check(parse(text, &command) == result, text);
}

int main() {
  TcCommand c;
  check(parse("HELLO", &c) == TC_PARSE_OK && c.type == TC_CMD_HELLO, "HELLO");
  check(parse("LIST", &c) == TC_PARSE_OK && c.type == TC_CMD_LIST, "LIST");
  check(parse("GETTZ", &c) == TC_PARSE_OK && c.type == TC_CMD_GETTZ, "GETTZ");
  check(parse("GETCFG", &c) == TC_PARSE_OK && c.type == TC_CMD_GETCFG, "GETCFG");
  check(parse("WIPE", &c) == TC_PARSE_OK && c.type == TC_CMD_WIPE, "WIPE");
  expect("HELLO now", TC_PARSE_BADARG);
  expect("WIPE ALL", TC_PARSE_BADARG);
  expect("hello", TC_PARSE_UNKNOWN);
  expect("FROB", TC_PARSE_UNKNOWN);
  expect("", TC_PARSE_UNKNOWN);

  check(parse("DEL 3", &c) == TC_PARSE_OK && c.type == TC_CMD_DEL && c.id == 3, "DEL 3");
  check(parse("DEL 19", &c) == TC_PARSE_OK && c.id == 19, "DEL 19");
  expect("DEL 20", TC_PARSE_BADARG);
  expect("DEL", TC_PARSE_BADARG);
  expect("DEL -1", TC_PARSE_BADARG);
  expect("DEL 1 2", TC_PARSE_BADARG);
  expect("DEL  1", TC_PARSE_BADARG);
  expect("DEL 1x", TC_PARSE_BADARG);

  check(parse("TOKEN 2 1700000000", &c) == TC_PARSE_OK && c.type == TC_CMD_TOKEN &&
            c.id == 2 && c.time == 1700000000UL,
        "TOKEN");
  check(parse("TOKEN 0 4294967295", &c) == TC_PARSE_OK && c.time == 4294967295UL,
        "TOKEN max time");
  expect("TOKEN 0 4294967296", TC_PARSE_BADARG);
  expect("TOKEN 0", TC_PARSE_BADARG);
  expect("TOKEN 0 12 13", TC_PARSE_BADARG);

  check(parse("SETTZ 8", &c) == TC_PARSE_OK && c.timezone == 8, "SETTZ +2h");
  check(parse("SETTZ -48", &c) == TC_PARSE_OK && c.timezone == -48, "SETTZ -12h");
  check(parse("SETTZ 56", &c) == TC_PARSE_OK && c.timezone == 56, "SETTZ +14h");
  expect("SETTZ 57", TC_PARSE_BADARG);
  expect("SETTZ -49", TC_PARSE_BADARG);
  expect("SETTZ", TC_PARSE_BADARG);
  expect("SETTZ +8", TC_PARSE_BADARG);

  check(parse("SETCFG 05", &c) == TC_PARSE_OK && c.config == 0x05, "SETCFG 05");
  check(parse("SETCFG 0f", &c) == TC_PARSE_OK && c.config == 0x0F, "SETCFG 0f");
  expect("SETCFG 10", TC_PARSE_BADARG);  // reserved bit
  expect("SETCFG 5", TC_PARSE_BADARG);
  expect("SETCFG zz", TC_PARSE_BADARG);

  check(parse("ADD MFRGGZDF SHA1 6 30 GitHub", &c) == TC_PARSE_OK &&
            c.type == TC_CMD_ADD && c.key.digits == 6 && c.key.period == 30 &&
            !strcmp(c.key.name, "GitHub") && !memcmp(c.key.secret, "abcde", 5) &&
            c.key.secret[5] == 0,
        "ADD");
  check(parse("ADD MFRGGZDF SHA1 8 60 Work: AWS root", &c) == TC_PARSE_OK &&
            c.key.digits == 8 && c.key.period == 60 && !strcmp(c.key.name, "Work: AWS root"),
        "ADD name with spaces");
  expect("ADD MFRGGZDF SHA1 6 30", TC_PARSE_BADARG);            // no name
  expect("ADD MFRGGZDF SHA1 6 30 ", TC_PARSE_BADARG);           // empty name
  expect("ADD MFRGGZDF SHA256 6 30 X", TC_PARSE_BADARG);        // algorithm
  expect("ADD MFRGGZDF SHA1 5 30 X", TC_PARSE_BADARG);          // digits
  expect("ADD MFRGGZDF SHA1 9 30 X", TC_PARSE_BADARG);
  expect("ADD MFRGGZDF SHA1 6 45 X", TC_PARSE_BADARG);          // period
  expect("ADD MFRG1ZDF SHA1 6 30 X", TC_PARSE_BADARG);          // base32
  expect("ADD GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQGEZDGNBV SHA1 6 30 X", TC_PARSE_BADARG);  // > 20 bytes
  expect("ADD MFRGGZDF SHA1 6 30 1234567890123456789012345678", TC_PARSE_BADARG);       // 28 chars
  check(parse("ADD GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ SHA1 6 30 Max", &c) == TC_PARSE_OK,
        "ADD with a 20 byte secret");

  // the parser does not validate name rules; the storage layer does
  check(parse("ADD MFRGGZDF SHA1 6 30 bad\x01name", &c) == TC_PARSE_OK &&
            !tc_name_valid(c.key.name),
        "name rules are left to tc_name_valid");

  printf(failures ? "%d FAILED\n" : "all protocol tests passed\n", failures);
  return failures != 0;
}
