// Host tests for the time helpers.
#include <stdio.h>
#include <string.h>

#include "../trustcore/src/tc_time.h"

static int failures;

static void check(bool ok, const char *what) {
  if (!ok) {
    failures++;
    printf("FAIL  %s\n", what);
  }
}

static void expectLocal(uint32_t unixTime, int8_t offset, const char *expected) {
  char text[24];
  tc_format_local(tc_local_time(unixTime, offset), offset, text);
  if (strcmp(text, expected)) {
    failures++;
    printf("FAIL  %u %d: got \"%s\", expected \"%s\"\n", unixTime, offset, text, expected);
  }
}

int main() {
  TcDateTime t = tc_local_time(0, 0);
  check(t.year == 1970 && t.month == 1 && t.day == 1 && t.hour == 0 && t.minute == 0, "epoch");
  t = tc_local_time(1700000000, 0);
  check(t.year == 2023 && t.month == 11 && t.day == 14 && t.hour == 22 &&
            t.minute == 13 && t.second == 20,
        "2023-11-14 22:13:20");
  t = tc_local_time(951782400, 0);
  check(t.year == 2000 && t.month == 2 && t.day == 29, "leap day 2000");
  t = tc_local_time(951868800, 0);
  check(t.year == 2000 && t.month == 3 && t.day == 1, "day after leap day 2000");
  t = tc_local_time(4294967295U, 0);
  check(t.year == 2106 && t.month == 2 && t.day == 7 && t.hour == 6 && t.minute == 28,
        "uint32 maximum");
  t = tc_local_time(0, -4);  // UTC-1 at the epoch: still 1969
  check(t.year == 1969 && t.month == 12 && t.day == 31 && t.hour == 23, "before the epoch");

  expectLocal(1700000000, 0, "14.11 22:13 +0");
  expectLocal(1700000000, 4, "14.11 23:13 +1");   // CET
  expectLocal(1700000000, 8, "15.11 00:13 +2");   // CEST crosses midnight
  expectLocal(1700000000, 23, "15.11 03:58+5:45");  // Nepal: the space is dropped (16 chars)
  expectLocal(1700000000, -38, "14.11 12:43-9:30");  // the space is dropped (16 chars)
  expectLocal(1700000000, -48, "14.11 10:13 -12");
  expectLocal(1700000000, 56, "15.11 12:13 +14");

  char text[16];
  tc_format_offset(8, true, text);
  check(!strcmp(text, "UTC+2"), "UTC+2");
  tc_format_offset(-14, true, text);
  check(!strcmp(text, "UTC-3:30"), "UTC-3:30");
  tc_format_offset(0, false, text);
  check(!strcmp(text, "+0"), "+0");

  printf(failures ? "%d FAILED\n" : "all time tests passed\n", failures);
  return failures != 0;
}
