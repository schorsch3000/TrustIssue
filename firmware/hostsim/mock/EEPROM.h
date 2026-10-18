#ifndef MOCK_EEPROM_H
#define MOCK_EEPROM_H
#include "hostsim.h"
struct EEPROMClass {
  uint8_t read(int address) { return hs_eeprom[address]; }
  // A real byte write takes about 3.3 ms; it is what makes the wipe progress bar visible.
  void update(int address, uint8_t value) {
    if (hs_eeprom[address] == value) return;
    hs_eeprom[address] = value;
    hs_advance(3);
  }
};
static EEPROMClass EEPROM;
#endif
