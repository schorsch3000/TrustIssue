#ifndef MOCK_AVR_BOOT_H
#define MOCK_AVR_BOOT_H
#include "../../hostsim.h"
#define GET_LOCK_BITS 1
#define GET_HIGH_FUSE_BITS 2
// A finished device has lock byte 0xCC and high fuse 0xDA (see checkLock in core.ino).
inline uint8_t boot_lock_fuse_bits_get(int which) {
  if (which == GET_LOCK_BITS) return hs_unlocked ? 0xCF : 0xCC;
  return 0xDA;
}
#endif
