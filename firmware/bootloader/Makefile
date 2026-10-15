# Trust Issues bootloader for the ATmega328P (16 MHz), 2 KB boot section at 0x7800.
AVR_BIN ?= $(firstword $(wildcard $(HOME)/.arduino15/packages/arduino/tools/avr-gcc/*/bin))
CC = $(AVR_BIN)/avr-gcc
OBJCOPY = $(AVR_BIN)/avr-objcopy
SIZE = $(AVR_BIN)/avr-size

BOOT_START = 0x7800
CFLAGS = -mmcu=atmega328p -Os -std=gnu99 -Wall -Wextra -DF_CPU=16000000UL \
         -ffunction-sections -fdata-sections -fno-jump-tables -mrelax -fno-inline-small-functions -fno-split-wide-types -fno-tree-ter
LDFLAGS = -nostartfiles -Wl,--gc-sections -Wl,--section-start=.text=$(BOOT_START)

all: boot.hex

boot_font.h: boot.c gen_font.py victoriamedium8.bin
	python3 -I gen_font.py boot.c victoriamedium8.bin $@

boot.elf: boot.c boot_font.h
	$(CC) $(CFLAGS) $(LDFLAGS) -o $@ boot.c

boot.hex: boot.elf
	$(OBJCOPY) -O ihex -R .eeprom -R .fuse -R .lock -R .signature $< $@
	$(SIZE) --mcu=atmega328p -C $<
	@python3 -I -c "import re,sys; n=sum(int(l[1:3],16) for l in open('$@') if l.startswith(':') and l[7:9]=='00'); print('boot section use:', n, 'of 2048 bytes'); sys.exit(0 if n<=2048 else 1)"

clean:
	rm -f boot.elf boot.hex boot_font.h
