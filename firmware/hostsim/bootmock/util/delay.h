#ifndef MOCK_BOOT_DELAY_H
#define MOCK_BOOT_DELAY_H
void hs_delay_ms(unsigned ms);
#define _delay_ms(ms) hs_delay_ms(ms)
#endif
