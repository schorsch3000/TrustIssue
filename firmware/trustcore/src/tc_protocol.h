#ifndef TC_PROTOCOL_H
#define TC_PROTOCOL_H

#include "tc_storage.h"

// Line parser for the host protocol (see CLAUDE.md, "Serial protocol").
// Commands are upper case, fields are separated by exactly one space, and the
// free-text name of ADD is always the last field.

enum TcCommandType : uint8_t {
  TC_CMD_HELLO,
  TC_CMD_LIST,
  TC_CMD_GETTZ,
  TC_CMD_GETCFG,
  TC_CMD_ADD,     // key
  TC_CMD_DEL,     // id
  TC_CMD_TOKEN,   // id, time
  TC_CMD_SETTZ,   // timezone
  TC_CMD_SETCFG,  // config
  TC_CMD_WIPE,
};

enum TcParseResult : uint8_t {
  TC_PARSE_OK = 0,
  TC_PARSE_UNKNOWN,  // reply "ERR UNKNOWN"
  TC_PARSE_BADARG,   // reply "ERR BADARG"
};

struct TcCommand {
  TcCommandType type;
  uint8_t id;
  uint32_t time;
  int8_t timezone;  // 15 minute units, -48 .. +56
  uint8_t config;
  TcKey key;
};

// Destroys the line (splits it in place). The line must not contain CR or LF.
TcParseResult tc_parse_command(char *line, TcCommand *command);

// The configuration bits the device understands; others are rejected.
#define TC_CONFIG_MASK 0x1F

#endif
