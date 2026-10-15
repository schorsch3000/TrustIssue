#ifndef CORE_TYPES_H
#define CORE_TYPES_H

#include <stdint.h>

enum ButtonEvent : uint8_t { BUTTON_NONE, BUTTON_PRESSED, BUTTON_RELEASED };
enum State : uint8_t { STATE_IDLE, STATE_PENDING, STATE_AUTOWIPE };
enum Outcome : uint8_t { OUTCOME_ACCEPTED, OUTCOME_DENIED, OUTCOME_TIMEOUT, OUTCOME_TOO_LONG };

#endif
