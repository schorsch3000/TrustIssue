#ifndef MOCK_BOOT_PGMSPACE_H
#define MOCK_BOOT_PGMSPACE_H
#include <stdint.h>
#define PROGMEM
// Pointers read the bootloader's own tables; plain numbers are flash addresses (PIN page, firmware).
#define pgm_read_byte(p) (*(const volatile uint8_t *)(p))
uint8_t hs_flash_byte(uint16_t address);
uint16_t hs_flash_word(uint16_t address);
#define pgm_read_byte_near(a) hs_flash_byte((uint16_t)(a))
#define pgm_read_word(a) hs_flash_word((uint16_t)(a))
#endif
