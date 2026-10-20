// The parts of trustctl-menubar that need no window system: settings, parsing trustctl's output, the filter and the
// hotkey text. Tested by tests/main.swift (../test.sh); Foundation only, so the tests build without AppKit.

import Foundation

// ---------------------------------------------------------------- settings
final class Settings {
    var trustctlPath = ""   // empty = look for trustctl in the usual places (a GUI app does not get the shell's PATH)
    var device = ""         // optional --device value (/dev/cu.usbserial-...); empty = trustctl's own configuration
    var hotkey = ""         // "Ctrl+Opt+T" or "Ctrl+Opt+code:17"; empty = no hotkey
    var action = "type"     // type | copy
    var pressEnter = false  // after typing
    var sort = false        // list the keys sorted by name instead of in the device's slot order
    var delayMs = 300       // before typing (time to let go of the hotkey)

    static var filePath: String {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        return base.appendingPathComponent("trustctl").appendingPathComponent("menubar.ini").path
    }

    static func load(_ file: String) -> Settings {
        let s = Settings()
        guard let text = try? String(contentsOfFile: file, encoding: .utf8) else { return s }
        for raw in text.replacingOccurrences(of: "\r", with: "").components(separatedBy: "\n") {
            let line = raw.trimmingCharacters(in: .whitespaces)
            guard !line.hasPrefix("#"), let eq = line.firstIndex(of: "="), eq != line.startIndex else { continue }
            let key = line[..<eq].trimmingCharacters(in: .whitespaces).lowercased()
            let value = line[line.index(after: eq)...].trimmingCharacters(in: .whitespaces)
            let yes = ["1", "true", "yes"].contains(value.lowercased())
            switch key {
            case "trustctl": s.trustctlPath = value
            case "device": s.device = value
            case "hotkey": s.hotkey = value
            case "action": if value == "type" || value == "copy" { s.action = value }
            case "enter": s.pressEnter = yes
            case "sort": s.sort = yes
            case "delay": if let n = Int(value), n >= 0, n <= 10000 { s.delayMs = n }
            default: break
            }
        }
        return s
    }

    func save(_ file: String) throws {
        try FileManager.default.createDirectory(atPath: (file as NSString).deletingLastPathComponent,
                                                withIntermediateDirectories: true)
        let text = "# trustctl-menubar settings\n" +
            "trustctl=\(trustctlPath)\n" +
            "device=\(device)\n" +
            "hotkey=\(hotkey)\n" +
            "action=\(action)\n" +
            "enter=\(pressEnter ? "1" : "0")\n" +
            "sort=\(sort ? "1" : "0")\n" +
            "delay=\(delayMs)\n"
        try text.write(toFile: file, atomically: true, encoding: .utf8)
    }
}

// ---------------------------------------------------------------- pure helpers
enum Logic {
    // "  3  GitHub" -> "GitHub": trustctl list prints the slot number, two spaces, then the name.
    static func parseList(_ output: String) -> [String] {
        var names: [String] = []
        for raw in output.replacingOccurrences(of: "\r", with: "").components(separatedBy: "\n") {
            let chars = Array(raw)
            var i = 0
            while i < chars.count && chars[i] == " " { i += 1 }
            let digits = i
            while i < chars.count && chars[i] >= "0" && chars[i] <= "9" { i += 1 }
            if i == digits || i + 2 > chars.count || chars[i] != " " || chars[i + 1] != " " { continue }
            let name = String(chars[(i + 2)...])
            if !name.isEmpty { names.append(name) }
        }
        return names
    }

    // By name, case ignored; keys whose names differ only in case keep the device's order.
    static func sortByName(_ names: [String]) -> [String] {
        return names.enumerated().sorted { a, b in
            let c = a.element.caseInsensitiveCompare(b.element)
            return c != .orderedSame ? c == .orderedAscending : a.offset < b.offset
        }.map { $0.element }
    }

    // The names that contain every word of the filter, case-insensitive; names that start with the first
    // word come first. An empty filter keeps everything in the given order.
    static func filter(_ names: [String], _ filter: String) -> [String] {
        let words = filter.lowercased().split(separator: " ").map(String.init)
        var starts: [String] = []
        var others: [String] = []
        for name in names {
            let lower = name.lowercased()
            if !words.allSatisfy({ lower.contains($0) }) { continue }
            if let first = words.first, lower.hasPrefix(first) { starts.append(name) } else { others.append(name) }
        }
        return starts + others
    }

    // The last non-empty line of the output (trustctl prints the code alone on stdout).
    static func lastLine(_ output: String) -> String {
        for line in output.replacingOccurrences(of: "\r", with: "").components(separatedBy: "\n").reversed() {
            let t = line.trimmingCharacters(in: .whitespaces)
            if !t.isEmpty { return t }
        }
        return ""
    }

    // A TOTP code: 6 to 8 digits (leading zeros included).
    static func isCode(_ text: String) -> Bool {
        return (6...8).contains(text.count) && text.allSatisfy { $0 >= "0" && $0 <= "9" }
    }

    // The trustctl to run: a configured path (with ~), or the first executable "trustctl" in PATH and in the
    // places Homebrew, go install and the release archives' instructions put it. nil if there is none.
    static func findTrustctl(configured: String, path: String, home: String,
                             isExecutable: (String) -> Bool) -> String? {
        var wanted = configured.trimmingCharacters(in: .whitespaces)
        if wanted.hasPrefix("~/") { wanted = home + String(wanted.dropFirst(1)) }
        if wanted.contains("/") { return isExecutable(wanted) ? wanted : nil }
        let name = wanted.isEmpty ? "trustctl" : wanted
        var dirs = path.split(separator: ":").map(String.init)
        dirs += ["/opt/homebrew/bin", "/usr/local/bin", home + "/bin", home + "/.local/bin", home + "/go/bin"]
        for dir in dirs where !dir.isEmpty {
            let candidate = (dir as NSString).appendingPathComponent(name)
            if isExecutable(candidate) { return candidate }
        }
        return nil
    }

    // ------------------------------------------------------------ hotkeys
    // Carbon modifier masks (cmdKey, shiftKey, optionKey, controlKey) as RegisterEventHotKey wants them.
    static let modCommand: UInt32 = 0x0100
    static let modShift: UInt32 = 0x0200
    static let modOption: UInt32 = 0x0800
    static let modControl: UInt32 = 0x1000

    // Virtual key codes by name. Letters and digits are named after their place on a US keyboard, which is
    // not what a German keyboard prints on them (Y and Z are swapped), so the settings window writes those as
    // code:N and only shows the key's name in the current layout. The names here are for editing the file.
    static let keyCodes: [String: UInt32] = [
        "A": 0x00, "S": 0x01, "D": 0x02, "F": 0x03, "H": 0x04, "G": 0x05, "Z": 0x06, "X": 0x07, "C": 0x08,
        "V": 0x09, "B": 0x0B, "Q": 0x0C, "W": 0x0D, "E": 0x0E, "R": 0x0F, "Y": 0x10, "T": 0x11, "1": 0x12,
        "2": 0x13, "3": 0x14, "4": 0x15, "6": 0x16, "5": 0x17, "9": 0x19, "7": 0x1A, "8": 0x1C, "0": 0x1D,
        "O": 0x1F, "U": 0x20, "I": 0x22, "P": 0x23, "L": 0x25, "J": 0x26, "K": 0x28, "N": 0x2D, "M": 0x2E,
        "Space": 0x31, "F1": 0x7A, "F2": 0x78, "F3": 0x63, "F4": 0x76, "F5": 0x60, "F6": 0x61, "F7": 0x62,
        "F8": 0x64, "F9": 0x65, "F10": 0x6D, "F11": 0x67, "F12": 0x6F, "F13": 0x69, "F14": 0x6B, "F15": 0x71,
        "F16": 0x6A, "F17": 0x40, "F18": 0x4F, "F19": 0x50,
    ]

    // Keys whose name does not depend on the keyboard layout.
    static func fixedKeyName(_ code: UInt32) -> String? {
        for (name, c) in keyCodes where c == code && (name == "Space" || name.hasPrefix("F") && name.count > 1) {
            return name
        }
        return nil
    }

    // "Ctrl+Opt+T" -> Carbon modifiers and a key code; nil if invalid. A hotkey needs Ctrl, Opt or Cmd:
    // Shift alone would take a capital letter away from every program.
    static func parseHotkey(_ text: String) -> (modifiers: UInt32, keyCode: UInt32)? {
        var mods: UInt32 = 0
        var key: UInt32?
        for part in text.split(separator: "+", omittingEmptySubsequences: false) {
            let p = part.trimmingCharacters(in: .whitespaces)
            switch p.lowercased() {
            case "ctrl", "control": mods |= modControl
            case "opt", "option", "alt": mods |= modOption
            case "shift": mods |= modShift
            case "cmd", "command": mods |= modCommand
            default:
                if key != nil { return nil }
                if p.lowercased().hasPrefix("code:"), let n = UInt32(p.dropFirst(5)), n < 128 { key = n; continue }
                guard let match = keyCodes.first(where: { $0.key.lowercased() == p.lowercased() }) else { return nil }
                key = match.value
            }
        }
        guard let k = key, mods & (modControl | modOption | modCommand) != 0 else { return nil }
        return (mods, k)
    }

    // The settings file form: Ctrl+Opt+Shift+Cmd in the order macOS menus show them.
    static func formatHotkey(modifiers: UInt32, keyCode: UInt32) -> String {
        var s = ""
        if modifiers & modControl != 0 { s += "Ctrl+" }
        if modifiers & modOption != 0 { s += "Opt+" }
        if modifiers & modShift != 0 { s += "Shift+" }
        if modifiers & modCommand != 0 { s += "Cmd+" }
        return s + (fixedKeyName(keyCode) ?? "code:\(keyCode)")
    }

    // What the settings window shows: ⌃⌥⇧⌘ and the key's name (the caller knows the keyboard layout).
    static func displayHotkey(modifiers: UInt32, keyName: String) -> String {
        var s = ""
        if modifiers & modControl != 0 { s += "⌃" }
        if modifiers & modOption != 0 { s += "⌥" }
        if modifiers & modShift != 0 { s += "⇧" }
        if modifiers & modCommand != 0 { s += "⌘" }
        return s + keyName
    }
}
