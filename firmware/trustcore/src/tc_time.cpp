#include "tc_time.h"

#include <string.h>

TcDateTime tc_local_time(uint32_t unixTime, int8_t offsetQuarterHours) {
  int64_t local = (int64_t)unixTime + (int32_t)offsetQuarterHours * 900;
  int32_t days = (int32_t)(local / 86400);
  int32_t seconds = (int32_t)(local % 86400);
  if (seconds < 0) {
    seconds += 86400;
    days--;
  }
  // civil-from-days (H. Hinnant), days since 1970-01-01
  int32_t z = days + 719468;
  int32_t era = (z >= 0 ? z : z - 146096) / 146097;
  int32_t dayOfEra = z - era * 146097;
  int32_t yearOfEra =
      (dayOfEra - dayOfEra / 1460 + dayOfEra / 36524 - dayOfEra / 146096) / 365;
  int32_t year = yearOfEra + era * 400;
  int32_t dayOfYear = dayOfEra - (365 * yearOfEra + yearOfEra / 4 - yearOfEra / 100);
  int32_t monthIndex = (5 * dayOfYear + 2) / 153;
  int32_t day = dayOfYear - (153 * monthIndex + 2) / 5 + 1;
  int32_t month = monthIndex < 10 ? monthIndex + 3 : monthIndex - 9;
  if (month <= 2) year++;

  TcDateTime result;
  result.year = year;
  result.month = month;
  result.day = day;
  result.hour = seconds / 3600;
  result.minute = (seconds % 3600) / 60;
  result.second = seconds % 60;
  return result;
}

static char *putNumber(char *out, uint16_t value, uint8_t minDigits) {
  char digits[5];
  uint8_t count = 0;
  do {
    digits[count++] = '0' + value % 10;
    value /= 10;
  } while (value);
  while (count < minDigits) digits[count++] = '0';
  while (count) *out++ = digits[--count];
  return out;
}

void tc_format_offset(int8_t offsetQuarterHours, bool withPrefix, char *out) {
  if (withPrefix) {
    memcpy(out, "UTC", 3);
    out += 3;
  }
  int16_t quarters = offsetQuarterHours;
  *out++ = quarters < 0 ? '-' : '+';
  if (quarters < 0) quarters = -quarters;
  out = putNumber(out, quarters / 4, 1);
  if (quarters % 4) {
    *out++ = ':';
    out = putNumber(out, (quarters % 4) * 15, 2);
  }
  *out = '\0';
}

void tc_format_local(const TcDateTime &time, char *out) {
  char *p = out;
  p = putNumber(p, time.day, 2);
  *p++ = '.';
  p = putNumber(p, time.month, 2);
  *p++ = '.';
  p = putNumber(p, time.year, 4);
  *p++ = ' ';
  p = putNumber(p, time.hour, 2);
  *p++ = ':';
  p = putNumber(p, time.minute, 2);
  *p = '\0';
}
