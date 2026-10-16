// Tests of the pure helpers of TrustctlTray.cs (no Windows needed): build and run with ../test.sh
using System;
using System.Collections.Generic;
using System.Windows.Forms;
using TrustctlTray;

static class LogicTests
{
    static int failures;

    static void Check(bool ok, string what)
    {
        if (!ok) failures++;
        Console.WriteLine((ok ? "PASS  " : "FAIL  ") + what);
    }

    static string Join(List<string> l) { return string.Join("|", l.ToArray()); }

    static int Main()
    {
        List<string> names = Logic.ParseList(" 0  GitHub\r\n 1  Test  two\n12  a-b\n\nnot a line\n 3 one space\n");
        Check(Join(names) == "GitHub|Test  two|a-b", "ParseList keeps the name after the slot and two spaces: " + Join(names));
        Check(Logic.ParseList("").Count == 0, "ParseList of nothing");

        List<string> keys = new List<string> { "Amazon", "GitHub", "GitLab", "my GitHub", "Paypal" };
        Check(Join(Logic.Filter(keys, "")) == "Amazon|GitHub|GitLab|my GitHub|Paypal", "empty filter keeps all, in order");
        Check(Join(Logic.Filter(keys, "git")) == "GitHub|GitLab|my GitHub", "a prefix match comes first, then the rest: " + Join(Logic.Filter(keys, "git")));
        Check(Join(Logic.Filter(keys, "GITH")) == "GitHub|my GitHub", "case-insensitive");
        Check(Join(Logic.Filter(keys, "my hub")) == "my GitHub", "every word must match");
        Check(Logic.Filter(keys, "zzz").Count == 0, "no match");

        Check(Logic.QuoteArgument("token") == "token", "plain argument unchanged");
        Check(Logic.QuoteArgument("my GitHub") == "\"my GitHub\"", "space is quoted");
        Check(Logic.QuoteArgument("say \"hi\"") == "\"say \\\"hi\\\"\"", "quote is escaped");
        Check(Logic.QuoteArgument("a\\") == "a\\", "a trailing backslash without a space stays");
        Check(Logic.QuoteArgument("a b\\") == "\"a b\\\\\"", "a trailing backslash before the closing quote is doubled");
        Check(Logic.QuoteArgument("x\\\"y") == "\"x\\\\\\\"y\"", "backslashes before a quote are doubled and the quote escaped");
        Check(Logic.QuoteArgument("") == "\"\"", "an empty argument");

        uint mods;
        Keys key;
        Check(Logic.ParseHotkey("Ctrl+Alt+T", out mods, out key) && mods == 3 && key == Keys.T, "Ctrl+Alt+T");
        Check(Logic.ParseHotkey("Win+Shift+F9", out mods, out key) && mods == 12 && key == Keys.F9, "Win+Shift+F9");
        Check(!Logic.ParseHotkey("T", out mods, out key), "a key without a modifier is refused");
        Check(!Logic.ParseHotkey("Ctrl+", out mods, out key), "no key");
        Check(!Logic.ParseHotkey("Ctrl+Nonsense", out mods, out key), "unknown key");
        Check(!Logic.ParseHotkey("", out mods, out key), "empty");
        Check(Logic.FormatHotkey(Keys.Control | Keys.Alt, Keys.T) == "Ctrl+Alt+T", "FormatHotkey");
        Check(Logic.ParseHotkey(Logic.FormatHotkey(Keys.Control | Keys.Shift, Keys.D5), out mods, out key) && mods == 6 && key == Keys.D5, "format and parse round trip");

        Check(Logic.EscapeForSendKeys("012345") == "012345", "digits are not changed for SendKeys");
        Check(Logic.EscapeForSendKeys("+^%~(){}[]") == "{+}{^}{%}{~}{(}{)}{{}{}}{[}{]}", "SendKeys specials are escaped");

        Settings s = new Settings();
        s.TrustctlPath = "C:\\Tools\\trustctl.exe";
        s.Device = "COM5";
        s.Hotkey = "Ctrl+Alt+T";
        s.Action = "copy";
        s.PressEnter = true;
        s.DelayMs = 450;
        string file = System.IO.Path.Combine(System.IO.Path.GetTempPath(), "trustctl-tray-test-" + Guid.NewGuid().ToString("N") + ".ini");
        s.Save(file);
        Settings back = Settings.Load(file);
        Check(back.TrustctlPath == s.TrustctlPath && back.Device == "COM5" && back.Hotkey == "Ctrl+Alt+T" && back.Action == "copy" && back.PressEnter && back.DelayMs == 450, "settings round trip");
        System.IO.File.WriteAllText(file, "action=evil\ndelay=99999\ntrustctl=\n#x=1\ngarbage\n");
        back = Settings.Load(file);
        Check(back.Action == "type" && back.DelayMs == 300 && back.TrustctlPath == "trustctl.exe", "bad values fall back to the defaults");
        System.IO.File.Delete(file);
        Check(Settings.Load(file).TrustctlPath == "trustctl.exe", "a missing file gives the defaults");

        Console.WriteLine(failures == 0 ? "ALL PASSED" : failures + " FAILED");
        return failures == 0 ? 0 : 1;
    }
}
