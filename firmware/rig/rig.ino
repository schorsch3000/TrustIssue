// Trust Issues test rig, runs on the Arduino Uno.
//
// Presses the Nano's button (Uno D4 through 1k to Nano D2) and pulls the
// Nano's reset line (Uno D10 to Nano RST) with exact timings.
// Both pins are only ever driven low or released (input), never high.
//
// Line protocol, 115200 baud, every command is answered with "OK" when done:
//   HELLO
//   PRESS | RELEASE            hold the button down / let go
//   SEQ <ms> <ms> ...          press for ms1, release for ms2, press for ms3, ...
//                              (always ends released)
//   RESET <ms>                 pull the Nano's reset line low for ms
//   RESETHOLD <pre> <post>     press, wait pre ms, reset 50 ms, keep pressing
//                              post ms after the reset ends, release

#include <Arduino.h>

static const uint8_t BUTTON_PIN = 4;
static const uint8_t RESET_PIN = 10;

static char line[96];
static uint8_t lineLength;

static void pull(uint8_t pin) {
  digitalWrite(pin, LOW);
  pinMode(pin, OUTPUT);
}

static void let_go(uint8_t pin) { pinMode(pin, INPUT); }

static void handleLine(char *text) {
  char *word = strtok(text, " ");
  if (!word) return;
  if (!strcmp(word, "HELLO")) {
    Serial.println(F("TrustIssue-Rig 0.1"));
    return;
  }
  if (!strcmp(word, "PRESS")) {
    pull(BUTTON_PIN);
  } else if (!strcmp(word, "RELEASE")) {
    let_go(BUTTON_PIN);
  } else if (!strcmp(word, "SEQ")) {
    bool down = true;
    while ((word = strtok(NULL, " "))) {
      if (down) pull(BUTTON_PIN); else let_go(BUTTON_PIN);
      delay(strtoul(word, NULL, 10));
      down = !down;
    }
    let_go(BUTTON_PIN);
  } else if (!strcmp(word, "RESET")) {
    char *ms = strtok(NULL, " ");
    pull(RESET_PIN);
    delay(ms ? strtoul(ms, NULL, 10) : 50);
    let_go(RESET_PIN);
  } else if (!strcmp(word, "RESETHOLD")) {
    char *pre = strtok(NULL, " ");
    char *post = strtok(NULL, " ");
    if (!pre || !post) {
      Serial.println(F("ERR badarg"));
      return;
    }
    pull(BUTTON_PIN);
    delay(strtoul(pre, NULL, 10));
    pull(RESET_PIN);
    delay(50);
    let_go(RESET_PIN);
    delay(strtoul(post, NULL, 10));
    let_go(BUTTON_PIN);
  } else {
    Serial.println(F("ERR unknown"));
    return;
  }
  Serial.println(F("OK"));
}

void setup() {
  Serial.begin(115200);
  let_go(BUTTON_PIN);
  let_go(RESET_PIN);
}

void loop() {
  while (Serial.available()) {
    char c = Serial.read();
    if (c == '\r') continue;
    if (c == '\n') {
      line[lineLength] = '\0';
      lineLength = 0;
      handleLine(line);
    } else if (lineLength < sizeof(line) - 1) {
      line[lineLength++] = c;
    }
  }
}
