#ifndef MOCK_BOOT_BOOT_H
#define MOCK_BOOT_BOOT_H
#define boot_page_erase(a) ((void)(a))
#define boot_page_fill(a, w) ((void)(a), (void)(w))
#define boot_page_write(a) ((void)(a))
#define boot_spm_busy_wait()
#define boot_rww_enable()
#endif
