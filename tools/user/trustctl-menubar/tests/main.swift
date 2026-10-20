// Tests of the pure helpers of Logic.swift (no window system needed): build and run with ../test.sh
import Foundation

var failures = 0

func check(_ ok: Bool, _ what: String) {
    if !ok { failures += 1 }
    print((ok ? "PASS  " : "FAIL  ") + what)
}

func join(_ l: [String]) -> String { l.joined(separator: "|") }

let names = Logic.parseList(" 0  GitHub\r\n 1  Test  two\n12  a-b\n\nnot a line\n 3 one space\n")
check(join(names) == "GitHub|Test  two|a-b", "parseList keeps the name after the slot and two spaces: " + join(names))
check(Logic.parseList("").isEmpty, "parseList of nothing")

let keys = ["Amazon", "GitHub", "GitLab", "my GitHub", "Paypal"]
check(join(Logic.filter(keys, "")) == "Amazon|GitHub|GitLab|my GitHub|Paypal", "empty filter keeps all, in order")
check(join(Logic.filter(keys, "git")) == "GitHub|GitLab|my GitHub", "a prefix match comes first, then the rest: " + join(Logic.filter(keys, "git")))
check(join(Logic.filter(keys, "GITH")) == "GitHub|my GitHub", "case-insensitive")
check(join(Logic.filter(keys, "my hub")) == "my GitHub", "every word must match")
check(Logic.filter(keys, "zzz").isEmpty, "no match")

let sorted = Logic.sortByName(["GitLab", "aws", "GitHub", "Amazon", "github"])
check(join(sorted) == "Amazon|aws|GitHub|github|GitLab", "sortByName ignores case and keeps the device order for equal names: " + join(sorted))
check(Logic.sortByName([]).isEmpty, "sortByName of nothing")

check(Logic.lastLine("012345\r\n") == "012345", "lastLine of a code")
check(Logic.lastLine("note\n\n654321\n\n") == "654321", "lastLine skips empty lines")
check(Logic.lastLine("") == "", "lastLine of nothing")
check(Logic.isCode("012345") && Logic.isCode("12345678"), "6 and 8 digits are a code")
check(!Logic.isCode("12345") && !Logic.isCode("123456789") && !Logic.isCode("12 456") && !Logic.isCode("abcdef"), "other strings are not")

var h = Logic.parseHotkey("Ctrl+Opt+T")
check(h != nil && h!.modifiers == Logic.modControl | Logic.modOption && h!.keyCode == 0x11, "Ctrl+Opt+T")
h = Logic.parseHotkey("cmd+shift+F9")
check(h != nil && h!.modifiers == Logic.modCommand | Logic.modShift && h!.keyCode == 0x65, "cmd+shift+F9")
h = Logic.parseHotkey("Ctrl+Alt+code:6")
check(h != nil && h!.modifiers == Logic.modControl | Logic.modOption && h!.keyCode == 6, "a key code, and Alt for Opt")
check(Logic.parseHotkey("T") == nil, "a key without a modifier is refused")
check(Logic.parseHotkey("Shift+T") == nil, "Shift alone is not enough")
check(Logic.parseHotkey("Ctrl+") == nil, "no key")
check(Logic.parseHotkey("Ctrl+Nonsense") == nil, "unknown key")
check(Logic.parseHotkey("Ctrl+T+U") == nil, "two keys")
check(Logic.parseHotkey("Ctrl+code:200") == nil, "a key code out of range")
check(Logic.parseHotkey("") == nil, "empty")
check(Logic.formatHotkey(modifiers: Logic.modCommand | Logic.modControl, keyCode: 0x11) == "Ctrl+Cmd+code:17", "formatHotkey writes letters as codes")
check(Logic.formatHotkey(modifiers: Logic.modOption, keyCode: 0x31) == "Opt+Space", "formatHotkey names Space and F keys")
let all = Logic.modControl | Logic.modOption | Logic.modShift | Logic.modCommand
check(Logic.formatHotkey(modifiers: all, keyCode: 0x7A) == "Ctrl+Opt+Shift+Cmd+F1", "modifier order")
check(Logic.displayHotkey(modifiers: all, keyName: "Z") == "⌃⌥⇧⌘Z", "displayHotkey uses the menu symbols")
for (m, k): (UInt32, UInt32) in [(Logic.modControl | Logic.modShift, 0x1D), (Logic.modOption, 0x6F), (Logic.modCommand, 0x06)] {
    let back = Logic.parseHotkey(Logic.formatHotkey(modifiers: m, keyCode: k))
    check(back != nil && back!.modifiers == m && back!.keyCode == k, "format and parse round trip \(Logic.formatHotkey(modifiers: m, keyCode: k))")
}

let exe: Set<String> = ["/opt/homebrew/bin/trustctl", "/Users/u/bin/trustctl", "/Users/u/tools/trustctl", "/usr/bin/tc"]
let isExe: (String) -> Bool = { exe.contains($0) }
check(Logic.findTrustctl(configured: "", path: "/usr/bin:/bin", home: "/Users/u", isExecutable: isExe) == "/opt/homebrew/bin/trustctl", "found in Homebrew's directory when it is not in PATH")
check(Logic.findTrustctl(configured: "", path: "/Users/u/bin:/usr/bin", home: "/Users/u", isExecutable: isExe) == "/Users/u/bin/trustctl", "PATH comes first")
check(Logic.findTrustctl(configured: "~/tools/trustctl", path: "", home: "/Users/u", isExecutable: isExe) == "/Users/u/tools/trustctl", "a configured path with ~")
check(Logic.findTrustctl(configured: "/nowhere/trustctl", path: "/opt/homebrew/bin", home: "/Users/u", isExecutable: isExe) == nil, "a configured path is not replaced by a search")
check(Logic.findTrustctl(configured: "tc", path: "/usr/bin", home: "/Users/u", isExecutable: isExe) == "/usr/bin/tc", "a configured bare name is searched")
check(Logic.findTrustctl(configured: "", path: "", home: "/Users/x", isExecutable: { _ in false }) == nil, "nothing found")

let s = Settings()
s.trustctlPath = "/Users/u/bin/trustctl"
s.device = "/dev/cu.usbserial-110"
s.hotkey = "Ctrl+Opt+code:17"
s.action = "copy"
s.pressEnter = true
s.delayMs = 450
s.sort = true
let file = NSTemporaryDirectory() + "trustctl-menubar-test-\(UUID().uuidString)/menubar.ini"
try! s.save(file)
var back = Settings.load(file)
check(back.trustctlPath == s.trustctlPath && back.device == s.device && back.hotkey == s.hotkey && back.action == "copy" && back.pressEnter && back.delayMs == 450 && back.sort, "settings round trip")
try! "action=evil\ndelay=99999\n#x=1\ngarbage\nenter=yes\n".write(toFile: file, atomically: true, encoding: .utf8)
back = Settings.load(file)
check(!back.sort && back.action == "type" && back.delayMs == 300 && back.trustctlPath == "" && back.pressEnter, "bad values fall back to the defaults")
try! FileManager.default.removeItem(atPath: (file as NSString).deletingLastPathComponent)
check(Settings.load(file).trustctlPath == "" && Settings.load(file).action == "type", "a missing file gives the defaults")

print(failures == 0 ? "ALL PASSED" : "\(failures) FAILED")
exit(failures == 0 ? 0 : 1)
