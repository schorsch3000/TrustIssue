// Fake ATmega328P registers for the host build of the bootloader (see bootsim.cpp). A register that
// the bootloader polls or writes with a side effect is a small object; the rest are plain variables.
#ifndef MOCK_AVR_IO_H
#define MOCK_AVR_IO_H
#include <stdint.h>

#define _BV(bit) (1 << (bit))
#define SPM_PAGESIZE 128
#define PC3 3
#define TWINT 7
#define TWSTA 5
#define TWSTO 4
#define TWEN 2
#define RXC0 7
#define UDRE0 5
#define U2X0 1
#define RXEN0 4
#define TXEN0 3
#define EEPE 1
#define EERE 0

struct HsReg {  // a register with behavior: bsim.cpp implements these
  uint8_t id;
  explicit HsReg(uint8_t i) : id(i) {}
  HsReg &operator=(uint8_t value);
  operator uint8_t() const;
  HsReg &operator|=(uint8_t value) { return *this = (uint8_t)(*this | value); }
};
extern HsReg hs_TWCR, hs_TWDR, hs_TWSR, hs_TWBR, hs_UCSR0A, hs_UCSR0B, hs_UDR0, hs_UBRR0L, hs_PINC, hs_EEDR;
#define TWCR hs_TWCR
#define TWDR hs_TWDR
#define TWSR hs_TWSR
#define TWBR hs_TWBR
#define UCSR0A hs_UCSR0A
#define UCSR0B hs_UCSR0B
#define UDR0 hs_UDR0
#define UBRR0L hs_UBRR0L
#define PINC hs_PINC
#define EEDR hs_EEDR
extern uint8_t hs_PORTC, hs_MCUSR, hs_EECR;
extern uint16_t hs_EEAR;
#define PORTC hs_PORTC
#define MCUSR hs_MCUSR
#define EECR hs_EECR
#define EEAR hs_EEAR
#endif
