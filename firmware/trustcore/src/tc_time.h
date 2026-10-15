#ifndef TC_TIME_H
#define TC_TIME_H

#include <stdint.h>

struct TcDateTime {
  uint16_t year;
  uint8_t month;   // 1..12
  uint8_t day;     // 1..31
  uint8_t hour;
  uint8_t minute;
  uint8_t second;
};

// Converts a unix time (UTC) plus a timezone offset in 15 minute units to a
// local calendar date and time (proleptic Gregorian calendar).
TcDateTime tc_local_time(uint32_t unixTime, int8_t offsetQuarterHours);

// Writes the offset as "+2", "-9:30" or "+5:45" (no "UTC" prefix) when
// withPrefix is false, or "UTC+2" when it is true. out needs 12 bytes.
void tc_format_offset(int8_t offsetQuarterHours, bool withPrefix, char *out);

// "DD.MM HH:MM +2" (at most 16 characters, the space before the offset is
// dropped if it would not fit). out needs 20 bytes.
void tc_format_local(const TcDateTime &time, int8_t offsetQuarterHours, char *out);

#endif
