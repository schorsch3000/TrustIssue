// trustctl-menubar: the macOS counterpart of tools/user/trustctl-tray (Windows) and tools/user/trustctl-menu.
//
// An icon in the menu bar with a settings window (path of trustctl, an optional global hotkey, what to do with
// the code). The hotkey or a click on the icon opens a small list of the key names with a text field that filters
// as you type, like dmenu; Return or a click on a name asks the device for its code (hold the button), and the
// code is typed into the app that was active before, or copied to the clipboard.
//
// Two source files, no dependencies besides what macOS ships; built by build.sh with the Swift compiler of the
// Command Line Tools (no Xcode project). The code never appears on a command line and is never written to a
// file; the settings file holds only the path, the hotkey and the options.

import AppKit
import ApplicationServices
import Carbon.HIToolbox

let bundleID = "de.trustissue.trustctl-menubar"

// ---------------------------------------------------------------- trustctl
struct TrustctlResult {
    var exitCode: Int32 = -1
    var output = ""
    var error = ""
    var started = false
}

enum Trustctl {
    static func executable(_ s: Settings) -> String? {
        let env = ProcessInfo.processInfo.environment
        return Logic.findTrustctl(configured: s.trustctlPath, path: env["PATH"] ?? "",
                                  home: NSHomeDirectory(),
                                  isExecutable: { FileManager.default.isExecutableFile(atPath: $0) })
    }

    // Runs trustctl and waits; call it off the main thread.
    static func run(_ s: Settings, _ args: [String], timeout: TimeInterval) -> TrustctlResult {
        var r = TrustctlResult()
        guard let exe = executable(s) else {
            r.error = s.trustctlPath.isEmpty
                ? "trustctl was not found. Set its path in the settings."
                : "\(s.trustctlPath) is not an executable file."
            return r
        }
        let p = Process()
        p.executableURL = URL(fileURLWithPath: exe)
        p.arguments = (s.device.isEmpty ? [] : ["--device", s.device]) + args
        p.standardInput = FileHandle.nullDevice  // trustctl never waits for input here
        let out = Pipe(), err = Pipe()
        p.standardOutput = out
        p.standardError = err
        do { try p.run() } catch {
            r.error = "cannot start \(exe): \(error.localizedDescription)"
            return r
        }
        r.started = true
        // read stderr on its own thread so that neither pipe can fill up and stall trustctl
        var errData = Data()
        let errDone = DispatchSemaphore(value: 0)
        DispatchQueue.global().async {
            errData = err.fileHandleForReading.readDataToEndOfFile()
            errDone.signal()
        }
        let killer = DispatchWorkItem { if p.isRunning { p.terminate() } }
        DispatchQueue.global().asyncAfter(deadline: .now() + timeout, execute: killer)
        let outData = out.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        killer.cancel()
        errDone.wait()
        if p.terminationReason == .uncaughtSignal {
            r.error = "trustctl did not finish in time"
            return r
        }
        r.output = String(decoding: outData, as: UTF8.self)
        r.error = String(decoding: errData, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        r.exitCode = p.terminationStatus
        return r
    }
}

// ---------------------------------------------------------------- global hotkey (Carbon, needs no permission)
final class Hotkey {
    private var ref: EventHotKeyRef?
    private var handler: EventHandlerRef?
    var pressed: () -> Void = {}

    init() {
        var spec = EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed))
        let me = Unmanaged.passUnretained(self).toOpaque()
        InstallEventHandler(GetApplicationEventTarget(), { _, _, user in
            let hotkey = Unmanaged<Hotkey>.fromOpaque(user!).takeUnretainedValue()
            DispatchQueue.main.async { hotkey.pressed() }
            return noErr
        }, 1, &spec, me, &handler)
    }

    func unregister() {
        if let r = ref { UnregisterEventHotKey(r) }
        ref = nil
    }

    // false if the system or another program has it already
    func register(modifiers: UInt32, keyCode: UInt32) -> Bool {
        unregister()
        let id = EventHotKeyID(signature: OSType(0x5452_5354), id: 1)  // 'TRST'
        return RegisterEventHotKey(keyCode, modifiers, id, GetApplicationEventTarget(), 0, &ref) == noErr
    }
}

// The name a key code has in the current keyboard layout ("Z" on a German keyboard where a US one has "Y").
func keyName(_ code: UInt32) -> String {
    if let fixed = Logic.fixedKeyName(code) { return fixed }
    guard let source = TISCopyCurrentKeyboardLayoutInputSource()?.takeRetainedValue(),
          let raw = TISGetInputSourceProperty(source, kTISPropertyUnicodeKeyLayoutData) else { return "code:\(code)" }
    let data = Unmanaged<CFData>.fromOpaque(raw).takeUnretainedValue() as Data
    var dead: UInt32 = 0
    var chars = [UniChar](repeating: 0, count: 4)
    var length = 0
    let status = data.withUnsafeBytes { buf -> OSStatus in
        UCKeyTranslate(buf.baseAddress!.assumingMemoryBound(to: UCKeyboardLayout.self), UInt16(code),
                       UInt16(kUCKeyActionDisplay), 0, UInt32(LMGetKbdType()),
                       OptionBits(kUCKeyTranslateNoDeadKeysBit), &dead, 4, &length, &chars)
    }
    let name = String(utf16CodeUnits: chars, count: length).trimmingCharacters(in: .whitespacesAndNewlines)
    return status == noErr && !name.isEmpty ? name.uppercased() : "code:\(code)"
}

func carbonModifiers(_ flags: NSEvent.ModifierFlags) -> UInt32 {
    var m: UInt32 = 0
    if flags.contains(.control) { m |= Logic.modControl }
    if flags.contains(.option) { m |= Logic.modOption }
    if flags.contains(.shift) { m |= Logic.modShift }
    if flags.contains(.command) { m |= Logic.modCommand }
    return m
}

// ---------------------------------------------------------------- the picker (dmenu-like)
final class KeyPanel: NSPanel {
    override var canBecomeKey: Bool { true }
}

final class Picker: NSObject, NSTextFieldDelegate, NSTableViewDataSource, NSTableViewDelegate, NSWindowDelegate {
    private let panel: KeyPanel
    private let field = NSTextField()
    private let table = NSTableView()
    private let all: [String]
    private var shown: [String] = []
    private var done: ((String?) -> Void)?

    init(names: [String], done: @escaping (String?) -> Void) {
        all = names
        self.done = done
        let rowHeight: CGFloat = 24
        let rows = CGFloat(min(max(names.count, 1), 10))
        let width: CGFloat = 420
        let height = 34 + rows * rowHeight + 6
        panel = KeyPanel(contentRect: NSRect(x: 0, y: 0, width: width, height: height),
                         styleMask: [.borderless], backing: .buffered, defer: false)
        super.init()
        panel.level = .floating
        panel.isReleasedWhenClosed = false
        panel.hasShadow = true
        panel.backgroundColor = .windowBackgroundColor
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        panel.delegate = self

        let content = panel.contentView!
        content.wantsLayer = true
        content.layer?.borderWidth = 1
        content.layer?.borderColor = NSColor.separatorColor.cgColor

        field.frame = NSRect(x: 6, y: height - 30, width: width - 12, height: 24)
        field.font = .systemFont(ofSize: 15)
        field.placeholderString = "Filter"
        field.focusRingType = .none
        field.delegate = self
        content.addSubview(field)

        let column = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("name"))
        column.width = width - 4
        table.addTableColumn(column)
        table.headerView = nil
        table.style = .plain  // the default inset style pads the rows, and the last one did not fit
        table.rowHeight = rowHeight
        table.intercellSpacing = NSSize(width: 0, height: 0)
        table.dataSource = self
        table.delegate = self
        table.target = self
        table.action = #selector(clicked)
        table.refusesFirstResponder = true
        let scroll = NSScrollView(frame: NSRect(x: 1, y: 4, width: width - 2, height: rows * rowHeight))
        scroll.documentView = table
        scroll.hasVerticalScroller = true
        scroll.drawsBackground = false
        content.addSubview(scroll)

        refill()
        // centred on the screen with the mouse, a quarter down
        let screen = NSScreen.screens.first { NSMouseInRect(NSEvent.mouseLocation, $0.frame, false) } ?? NSScreen.main!
        let area = screen.visibleFrame
        panel.setFrameOrigin(NSPoint(x: area.midX - width / 2, y: area.maxY - area.height / 4 - height))
    }

    func show() {
        NSApp.activate(ignoringOtherApps: true)
        panel.makeKeyAndOrderFront(nil)
        panel.makeFirstResponder(field)
    }

    private func finish(_ chosen: String?) {
        guard let d = done else { return }
        done = nil
        panel.orderOut(nil)
        d(chosen)
    }

    private func refill() {
        shown = Logic.filter(all, field.stringValue)
        table.reloadData()
        if !shown.isEmpty { table.selectRowIndexes(IndexSet(integer: 0), byExtendingSelection: false) }
    }

    private func move(_ delta: Int) {
        guard !shown.isEmpty else { return }
        let row = max(0, min(shown.count - 1, table.selectedRow + delta))
        table.selectRowIndexes(IndexSet(integer: row), byExtendingSelection: false)
        table.scrollRowToVisible(row)
    }

    @objc private func clicked() {
        if table.clickedRow >= 0 { finish(shown[table.clickedRow]) }
    }

    func controlTextDidChange(_ obj: Notification) { refill() }

    func control(_ control: NSControl, textView: NSTextView, doCommandBy selector: Selector) -> Bool {
        switch selector {
        case #selector(NSResponder.cancelOperation(_:)): finish(nil)
        case #selector(NSResponder.insertNewline(_:)):
            if table.selectedRow >= 0 { finish(shown[table.selectedRow]) }
        case #selector(NSResponder.moveDown(_:)): move(1)
        case #selector(NSResponder.moveUp(_:)): move(-1)
        case #selector(NSResponder.pageDown(_:)), #selector(NSResponder.scrollPageDown(_:)): move(9)
        case #selector(NSResponder.pageUp(_:)), #selector(NSResponder.scrollPageUp(_:)): move(-9)
        default: return false
        }
        return true
    }

    func windowDidResignKey(_ notification: Notification) { finish(nil) }

    func numberOfRows(in tableView: NSTableView) -> Int { shown.count }

    func tableView(_ tableView: NSTableView, viewFor tableColumn: NSTableColumn?, row: Int) -> NSView? {
        let label = NSTextField(labelWithString: shown[row])
        label.font = .systemFont(ofSize: 15)
        label.lineBreakMode = .byTruncatingTail
        let cell = NSTableCellView()
        cell.addSubview(label)
        label.frame = NSRect(x: 8, y: 2, width: tableView.bounds.width - 16, height: 20)
        label.autoresizingMask = [.width]
        return cell
    }
}

// ---------------------------------------------------------------- a small notice under the menu bar
// macOS has no balloon tips, and system notifications need a signed app and a permission; this panel takes no
// focus, so typing into the previous app still works while it is shown.
final class Notice {
    private var panel: NSPanel?
    private var hideWork: DispatchWorkItem?

    func show(_ text: String, error: Bool = false, seconds: Double? = nil) {
        hide()
        let label = NSTextField(wrappingLabelWithString: text)
        label.font = .systemFont(ofSize: 13)
        label.textColor = error ? .systemRed : .labelColor
        label.preferredMaxLayoutWidth = 340
        let size = label.fittingSize
        let p = NSPanel(contentRect: NSRect(x: 0, y: 0, width: size.width + 24, height: size.height + 20),
                        styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
        p.level = .statusBar
        p.isReleasedWhenClosed = false
        p.hasShadow = true
        p.backgroundColor = .windowBackgroundColor
        p.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        p.ignoresMouseEvents = false
        label.frame = NSRect(x: 12, y: 10, width: size.width, height: size.height)
        p.contentView!.addSubview(label)
        p.contentView!.addGestureRecognizer(NSClickGestureRecognizer(target: self, action: #selector(clicked)))
        if let area = (NSScreen.main ?? NSScreen.screens.first)?.visibleFrame {
            p.setFrameOrigin(NSPoint(x: area.maxX - p.frame.width - 12, y: area.maxY - p.frame.height - 8))
        }
        p.orderFrontRegardless()
        panel = p
        if let s = seconds {
            let w = DispatchWorkItem { [weak self] in self?.hide() }
            hideWork = w
            DispatchQueue.main.asyncAfter(deadline: .now() + s, execute: w)
        }
    }

    @objc private func clicked() { hide() }

    func hide() {
        hideWork?.cancel()
        hideWork = nil
        panel?.orderOut(nil)
        panel = nil
    }
}

// ---------------------------------------------------------------- settings window
final class HotkeyField: NSTextField {
    var modifiers: UInt32 = 0
    var keyCode: UInt32 = 0
    var isSet = false

    override var acceptsFirstResponder: Bool { true }
    override func becomeFirstResponder() -> Bool { needsDisplay = true; return true }

    override func mouseDown(with event: NSEvent) { window?.makeFirstResponder(self) }

    func set(_ text: String) {
        if let h = Logic.parseHotkey(text) {
            modifiers = h.modifiers
            keyCode = h.keyCode
            isSet = true
            stringValue = Logic.displayHotkey(modifiers: modifiers, keyName: keyName(keyCode))
        } else {
            isSet = false
            stringValue = ""
        }
    }

    var settingText: String { isSet ? Logic.formatHotkey(modifiers: modifiers, keyCode: keyCode) : "" }

    override func performKeyEquivalent(with event: NSEvent) -> Bool {
        guard window?.firstResponder === self else { return false }
        keyDown(with: event)
        return true
    }

    override func keyDown(with event: NSEvent) {
        let code = UInt32(event.keyCode)
        if code == UInt32(kVK_Delete) || code == UInt32(kVK_ForwardDelete) { isSet = false; stringValue = ""; return }
        if code == UInt32(kVK_Tab) { window?.selectNextKeyView(self); return }
        let mods = carbonModifiers(event.modifierFlags)
        // a hotkey without Ctrl, Opt or Cmd would take a plain key away from every program
        guard mods & (Logic.modControl | Logic.modOption | Logic.modCommand) != 0 else { NSSound.beep(); return }
        modifiers = mods
        keyCode = code
        isSet = true
        stringValue = Logic.displayHotkey(modifiers: mods, keyName: keyName(code))
    }
}

final class SettingsWindow: NSObject, NSWindowDelegate {
    private let window: NSWindow
    private let path = NSTextField()
    private let device = NSTextField()
    private let hotkey = HotkeyField()
    private let sort = NSButton(checkboxWithTitle: "Sort the keys by name", target: nil, action: nil)
    private let type = NSButton(radioButtonWithTitle: "Type it into the app that was active before", target: nil, action: nil)
    private let enter = NSButton(checkboxWithTitle: "Then press Return", target: nil, action: nil)
    private let delay = NSTextField()
    private let copy = NSButton(radioButtonWithTitle: "Copy it to the clipboard", target: nil, action: nil)
    private let autostart = NSButton(checkboxWithTitle: "Start at login", target: nil, action: nil)
    private(set) var result: Settings?
    private(set) var autostartWanted = false

    init(_ s: Settings) {
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 560, height: 400),
                          styleMask: [.titled, .closable], backing: .buffered, defer: false)
        super.init()
        window.title = "trustctl-menubar settings"
        window.isReleasedWhenClosed = false
        window.delegate = self

        // Auto Layout throughout: every row takes the height its text needs, so nothing can overlap
        func hint(_ text: String) -> NSTextField {
            let l = NSTextField(wrappingLabelWithString: text)
            l.font = .systemFont(ofSize: NSFont.smallSystemFontSize)
            l.textColor = .secondaryLabelColor
            l.preferredMaxLayoutWidth = 380
            return l
        }
        func column(_ views: [NSView], spacing: CGFloat = 6) -> NSStackView {
            let st = NSStackView(views: views)
            st.orientation = .vertical
            st.alignment = .leading
            st.spacing = spacing
            return st
        }
        func row(_ views: [NSView]) -> NSStackView {
            let st = NSStackView(views: views)
            st.orientation = .horizontal
            st.alignment = .firstBaseline
            st.spacing = 8
            return st
        }
        func indented(_ v: NSView) -> NSView {
            let st = NSStackView(views: [v])
            st.edgeInsets = NSEdgeInsets(top: 0, left: 20, bottom: 0, right: 0)
            return st
        }
        func width(_ v: NSView, _ w: CGFloat) {
            v.widthAnchor.constraint(equalToConstant: w).isActive = true
        }

        path.stringValue = s.trustctlPath
        path.placeholderString = "found by itself"
        width(path, 280)
        let browse = NSButton(title: "Choose…", target: self, action: #selector(choose))
        let pathCell = column([row([path, browse]),
                               hint("Empty: the PATH, /opt/homebrew/bin, /usr/local/bin, ~/bin, ~/.local/bin and ~/go/bin are searched.")])

        device.stringValue = s.device
        device.placeholderString = "/dev/cu.usbserial-…"
        width(device, 280)
        let deviceCell = column([device,
                                 hint("Empty uses trustctl's own setting (TRUSTCTL_DEVICE does not reach an app, the config file does).")])

        hotkey.isEditable = false
        hotkey.isSelectable = false
        hotkey.isBezeled = true
        hotkey.drawsBackground = true
        hotkey.placeholderString = "none"
        hotkey.set(s.hotkey)
        width(hotkey, 160)
        let hotkeyCell = column([hotkey,
                                 hint("Click the field, then press the keys (Ctrl, Opt or Cmd is needed). Delete clears it.")])

        let grid = NSGridView(views: [
            [NSTextField(labelWithString: "Path of trustctl"), pathCell],
            [NSTextField(labelWithString: "Device (optional)"), deviceCell],
            [NSTextField(labelWithString: "Hotkey (optional)"), hotkeyCell],
        ])
        grid.rowSpacing = 14
        grid.columnSpacing = 10
        grid.column(at: 0).xPlacement = .trailing
        // the captions sit level with the first line of their field
        for i in 0..<grid.numberOfRows { grid.row(at: i).rowAlignment = .firstBaseline }

        sort.state = s.sort ? .on : .off

        // what belongs to typing hangs under it: the Return key and the wait before the first key
        type.target = self
        type.action = #selector(actionChanged)
        copy.target = self
        copy.action = #selector(actionChanged)
        enter.state = s.pressEnter ? .on : .off
        delay.integerValue = s.delayMs
        width(delay, 60)
        type.state = s.action == "type" ? .on : .off
        copy.state = s.action == "copy" ? .on : .off
        let waitRow = row([NSTextField(labelWithString: "Wait"), delay,
                           NSTextField(labelWithString: "ms before typing (time to let go of the hotkey)")])
        let box = NSBox()
        box.title = "When the device has given the code"
        let boxContent = column([type, indented(enter), indented(waitRow), copy], spacing: 8)
        boxContent.translatesAutoresizingMaskIntoConstraints = false
        box.contentView!.addSubview(boxContent)
        NSLayoutConstraint.activate([
            boxContent.topAnchor.constraint(equalTo: box.contentView!.topAnchor, constant: 8),
            boxContent.bottomAnchor.constraint(equalTo: box.contentView!.bottomAnchor, constant: -10),
            boxContent.leadingAnchor.constraint(equalTo: box.contentView!.leadingAnchor, constant: 10),
            boxContent.trailingAnchor.constraint(lessThanOrEqualTo: box.contentView!.trailingAnchor, constant: -10),
        ])

        autostart.state = LoginItem.enabled ? .on : .off

        let ok = NSButton(title: "OK", target: self, action: #selector(accept))
        ok.keyEquivalent = "\r"
        let cancel = NSButton(title: "Cancel", target: self, action: #selector(cancelled))
        cancel.keyEquivalent = "\u{1b}"
        width(ok, 90)
        width(cancel, 90)
        let spacer = NSView()
        spacer.setContentHuggingPriority(.defaultLow, for: .horizontal)
        let buttons = NSStackView(views: [spacer, cancel, ok])
        buttons.orientation = .horizontal

        let all = column([grid, sort, box, autostart, buttons], spacing: 16)
        all.edgeInsets = NSEdgeInsets(top: 20, left: 20, bottom: 20, right: 20)
        all.translatesAutoresizingMaskIntoConstraints = false
        let v = window.contentView!
        v.addSubview(all)
        NSLayoutConstraint.activate([
            all.topAnchor.constraint(equalTo: v.topAnchor),
            all.bottomAnchor.constraint(equalTo: v.bottomAnchor),
            all.leadingAnchor.constraint(equalTo: v.leadingAnchor),
            all.trailingAnchor.constraint(equalTo: v.trailingAnchor),
            box.widthAnchor.constraint(equalTo: all.widthAnchor, constant: -40),
            buttons.widthAnchor.constraint(equalTo: all.widthAnchor, constant: -40),
        ])
        actionChanged()
        window.layoutIfNeeded()
        window.setContentSize(all.fittingSize)
        window.center()
    }

    // Modal; returns true if the user pressed OK.
    func run() -> Bool {
        NSApp.activate(ignoringOtherApps: true)
        window.makeKeyAndOrderFront(nil)
        return NSApp.runModal(for: window) == .OK
    }

    @objc private func choose() {
        let d = NSOpenPanel()
        d.canChooseDirectories = false
        d.allowsMultipleSelection = false
        d.showsHiddenFiles = true
        if d.runModal() == .OK, let url = d.url { path.stringValue = url.path }
    }

    @objc private func actionChanged() {
        enter.isEnabled = type.state == .on
        delay.isEnabled = type.state == .on
    }

    @objc private func accept() {
        let s = Settings()
        s.trustctlPath = path.stringValue.trimmingCharacters(in: .whitespaces)
        s.device = device.stringValue.trimmingCharacters(in: .whitespaces)
        s.hotkey = hotkey.settingText
        s.action = copy.state == .on ? "copy" : "type"
        s.pressEnter = enter.state == .on
        s.sort = sort.state == .on
        s.delayMs = max(0, min(10000, delay.integerValue))
        result = s
        autostartWanted = autostart.state == .on
        NSApp.stopModal(withCode: .OK)
        window.orderOut(nil)
    }

    @objc private func cancelled() {
        NSApp.stopModal(withCode: .cancel)
        window.orderOut(nil)
    }

    func windowWillClose(_ notification: Notification) { NSApp.stopModal(withCode: .cancel) }
}

// ---------------------------------------------------------------- start at login (a LaunchAgent of the user)
enum LoginItem {
    static var file: String { NSHomeDirectory() + "/Library/LaunchAgents/\(bundleID).plist" }

    static var enabled: Bool { FileManager.default.fileExists(atPath: file) }

    static func set(_ on: Bool) throws {
        if !on {
            if enabled { try FileManager.default.removeItem(atPath: file) }
            return
        }
        // open -a starts the bundle, so it shows up as the app (and the Accessibility permission applies)
        let plist: [String: Any] = [
            "Label": bundleID,
            "ProgramArguments": ["/usr/bin/open", "-a", Bundle.main.bundlePath],
            "RunAtLoad": true,
        ]
        let data = try PropertyListSerialization.data(fromPropertyList: plist, format: .xml, options: 0)
        try FileManager.default.createDirectory(atPath: (file as NSString).deletingLastPathComponent,
                                                withIntermediateDirectories: true)
        try data.write(to: URL(fileURLWithPath: file))
    }
}

// ---------------------------------------------------------------- typing and the clipboard
enum Output {
    // Typing into another app needs the Accessibility permission; prompt asks macOS to show its dialog.
    static func mayType(prompt: Bool) -> Bool {
        let key = kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String
        return AXIsProcessTrustedWithOptions([key: prompt] as CFDictionary)
    }

    // Each character as a Unicode key event, so the keyboard layout does not matter.
    static func type(_ text: String, enter: Bool) {
        let source = CGEventSource(stateID: .hidSystemState)
        for unit in text.utf16 {
            var c = unit
            for down in [true, false] {
                let e = CGEvent(keyboardEventSource: source, virtualKey: 0, keyDown: down)
                e?.flags = []
                e?.keyboardSetUnicodeString(stringLength: 1, unicodeString: &c)
                e?.post(tap: .cghidEventTap)
            }
            usleep(8000)
        }
        if enter {
            for down in [true, false] {
                let e = CGEvent(keyboardEventSource: source, virtualKey: CGKeyCode(kVK_Return), keyDown: down)
                e?.flags = []
                e?.post(tap: .cghidEventTap)
            }
        }
    }

    static func copy(_ text: String) {
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setString(text, forType: .string)
        // asks clipboard managers not to keep it in their history (nspasteboard.org)
        pb.setString("", forType: NSPasteboard.PasteboardType("org.nspasteboard.ConcealedType"))
    }
}

// ---------------------------------------------------------------- the application (menu bar icon, hotkey)
final class App: NSObject, NSApplicationDelegate {
    private var item: NSStatusItem!
    private let menu = NSMenu()
    private let hotkey = Hotkey()
    private let notice = Notice()
    private var settings = Settings.load(Settings.filePath)
    private var picker: Picker?
    private var busy = false

    func applicationDidFinishLaunching(_ n: Notification) {
        if NSRunningApplication.runningApplications(withBundleIdentifier: bundleID).count > 1 {
            NSApp.terminate(nil)  // already running: its icon is in the menu bar
            return
        }
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = item.button {
            if let image = NSImage(systemSymbolName: "key.horizontal.fill", accessibilityDescription: "trustctl") {
                image.isTemplate = true
                button.image = image
            } else {
                button.title = "T"
            }
            button.toolTip = "trustctl"
            button.target = self
            button.action = #selector(iconClicked)
            button.sendAction(on: [.leftMouseUp, .rightMouseUp])
        }
        menu.addItem(withTitle: "Get a Code…", action: #selector(showPicker), keyEquivalent: "").target = self
        menu.addItem(withTitle: "Settings…", action: #selector(showSettings), keyEquivalent: "").target = self
        menu.addItem(.separator())
        menu.addItem(withTitle: "Quit trustctl-menubar", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "")

        hotkey.pressed = { [weak self] in self?.showPicker() }
        registerHotkey()
        if !FileManager.default.fileExists(atPath: Settings.filePath) {
            DispatchQueue.main.async { self.showSettings() }  // first start: ask for the path of trustctl
        }
    }

    @objc private func iconClicked() {
        let event = NSApp.currentEvent
        if event?.type == .rightMouseUp || event?.modifierFlags.contains(.control) == true {
            item.menu = menu  // shows it once, at the icon
            item.button?.performClick(nil)
            item.menu = nil
        } else {
            showPicker()
        }
    }

    private func registerHotkey() {
        hotkey.unregister()
        if settings.hotkey.isEmpty { return }
        guard let h = Logic.parseHotkey(settings.hotkey) else {
            notice.show("The hotkey \"\(settings.hotkey)\" is not valid.", error: true, seconds: 6)
            return
        }
        if !hotkey.register(modifiers: h.modifiers, keyCode: h.keyCode) {
            notice.show("The hotkey \(settings.hotkey) is taken by another program.", error: true, seconds: 6)
        }
    }

    private func error(_ text: String) { notice.show(text, error: true, seconds: 8) }

    @objc private func showSettings() {
        hotkey.unregister()  // so that the settings window can capture the same keys again
        let w = SettingsWindow(settings)
        if w.run(), let s = w.result {
            settings = s
            do {
                try s.save(Settings.filePath)
                try LoginItem.set(w.autostartWanted)
            } catch {
                self.error("Could not save the settings: \(error.localizedDescription)")
            }
            if s.action == "type" && !Output.mayType(prompt: true) {
                notice.show("To type the code, allow trustctl-menubar in System Settings > Privacy & Security > Accessibility.", seconds: 12)
            }
        }
        registerHotkey()
    }

    @objc private func showPicker() {
        if busy || picker != nil { return }  // the device is waiting for a button press, or the list is open
        let previous = NSWorkspace.shared.frontmostApplication
        busy = true
        let s = settings
        DispatchQueue.global().async {
            let list = Trustctl.run(s, ["list"], timeout: 20)
            DispatchQueue.main.async { self.listed(list, previous: previous) }
        }
    }

    private func listed(_ list: TrustctlResult, previous: NSRunningApplication?) {
        busy = false
        if !list.started || list.exitCode != 0 {
            error(list.error.isEmpty ? "trustctl list failed (exit \(list.exitCode))" : list.error)
            return
        }
        var names = Logic.parseList(list.output)
        if settings.sort { names = Logic.sortByName(names) }
        if names.isEmpty {
            error("The device holds no keys.")
            return
        }
        let p = Picker(names: names) { [weak self] chosen in
            guard let self = self else { return }
            self.picker = nil
            if let name = chosen { self.getToken(name, previous: previous) } else { previous?.activate() }
        }
        picker = p
        p.show()
    }

    private func getToken(_ name: String, previous: NSRunningApplication?) {
        busy = true
        previous?.activate()  // give the focus back now; the notice does not take it
        notice.show("Hold the button on the device for 1 to 2 seconds for \(name), then let go.")
        let s = settings
        // the device waits up to 30 s for the button; do not block the main thread meanwhile
        DispatchQueue.global().async {
            let r = Trustctl.run(s, ["token", name], timeout: 60)
            DispatchQueue.main.async { self.deliver(r, name: name, previous: previous) }
        }
    }

    private func deliver(_ r: TrustctlResult, name: String, previous: NSRunningApplication?) {
        busy = false
        notice.hide()
        let code = Logic.lastLine(r.output)
        if r.exitCode == 0 && !Logic.isCode(code) {
            let out = r.output.trimmingCharacters(in: .whitespacesAndNewlines)
            error("trustctl printed something unexpected instead of a code: " + (out.count > 60 ? out.prefix(60) + "…" : out))
            return
        }
        if r.exitCode != 0 || code.isEmpty {
            error(r.exitCode == 2 ? "Not confirmed on the device (denied or timed out)."
                  : r.error.isEmpty ? "trustctl token failed (exit \(r.exitCode))" : r.error)
            return
        }
        if settings.action == "copy" {
            Output.copy(code)
            notice.show("The code for \(name) is in the clipboard.", seconds: 2)
            return
        }
        guard Output.mayType(prompt: true) else {
            error("Typing needs the Accessibility permission: System Settings > Privacy & Security > Accessibility. The code was not typed.")
            return
        }
        previous?.activate()
        let enter = settings.pressEnter
        DispatchQueue.main.asyncAfter(deadline: .now() + .milliseconds(max(settings.delayMs, 50))) {
            Output.type(code, enter: enter)
        }
    }
}

@main
enum Main {
    static func main() {
        let app = NSApplication.shared
        app.setActivationPolicy(.accessory)  // no Dock icon, no menu of its own; Info.plist says the same
        let delegate = App()
        app.delegate = delegate
        app.run()
    }
}
