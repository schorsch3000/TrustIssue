#ifndef MOCK_BOOT_WDT_H
#define MOCK_BOOT_WDT_H
#define WDTO_15MS 15
#define WDTO_2S 2000
#define WDTO_8S 8000
void hs_wdt_enable(unsigned ms);
void hs_wdt_disable();
void hs_wdt_reset();
#define wdt_enable(ms) hs_wdt_enable(ms)
#define wdt_disable() hs_wdt_disable()
#define wdt_reset() hs_wdt_reset()
#endif
