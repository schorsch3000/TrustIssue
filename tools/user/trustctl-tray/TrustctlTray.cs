// trustctl-tray: the Windows counterpart of tools/user/trustctl-menu.
//
// A tray icon with a settings dialog (path of trustctl.exe, an optional global hotkey, what to do with the
// code). The hotkey or a click on the icon opens a small list of the key names with a text field that filters
// as you type, like dmenu; Enter or a click on a name asks the device for its code (hold the button), and the
// code is typed into the window that was active before, or copied to the clipboard.
//
// One file, no dependencies besides the .NET Framework that every Windows has. Written in C# 5 so that the
// compiler that comes with Windows can build it: build.cmd. The code never appears on a command line and is
// never written to a file; the settings file holds only the path, the hotkey and the options.

using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using System.Windows.Forms;

namespace TrustctlTray
{
    // ---------------------------------------------------------------- settings
    class Settings
    {
        public string TrustctlPath = "trustctl.exe";
        public string Device = "";          // optional --device value (a COM port); empty = trustctl's own configuration
        public string Hotkey = "";          // "Ctrl+Alt+T"; empty = no hotkey
        public string Action = "type";      // type | copy
        public bool PressEnter = false;     // after typing
        public int DelayMs = 300;           // before typing (time to let go of the hotkey)

        public static string FilePath
        {
            get
            {
                return Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "trustctl", "tray.ini");
            }
        }

        public static Settings Load(string file)
        {
            Settings s = new Settings();
            if (!File.Exists(file)) return s;
            foreach (string raw in File.ReadAllLines(file))
            {
                string line = raw.Trim();
                int eq = line.IndexOf('=');
                if (line.Length == 0 || line[0] == '#' || eq < 1) continue;
                string key = line.Substring(0, eq).Trim().ToLowerInvariant();
                string value = line.Substring(eq + 1).Trim();
                int number;
                switch (key)
                {
                    case "trustctl": if (value.Length > 0) s.TrustctlPath = value; break;
                    case "device": s.Device = value; break;
                    case "hotkey": s.Hotkey = value; break;
                    case "action": if (value == "type" || value == "copy") s.Action = value; break;
                    case "enter": s.PressEnter = value == "1" || value.ToLowerInvariant() == "true" || value.ToLowerInvariant() == "yes"; break;
                    case "delay": if (int.TryParse(value, out number) && number >= 0 && number <= 10000) s.DelayMs = number; break;
                }
            }
            return s;
        }

        public void Save(string file)
        {
            Directory.CreateDirectory(Path.GetDirectoryName(file));
            File.WriteAllText(file,
                "# trustctl-tray settings\r\n" +
                "trustctl=" + TrustctlPath + "\r\n" +
                "device=" + Device + "\r\n" +
                "hotkey=" + Hotkey + "\r\n" +
                "action=" + Action + "\r\n" +
                "enter=" + (PressEnter ? "1" : "0") + "\r\n" +
                "delay=" + DelayMs + "\r\n");
        }
    }

    // ---------------------------------------------------------------- pure helpers (tested in tests/)
    static class Logic
    {
        // "  3  GitHub" -> "GitHub": trustctl list prints the slot number, two spaces, then the name.
        public static List<string> ParseList(string output)
        {
            List<string> names = new List<string>();
            foreach (string raw in output.Replace("\r", "").Split('\n'))
            {
                int i = 0;
                while (i < raw.Length && raw[i] == ' ') i++;
                int digits = i;
                while (i < raw.Length && raw[i] >= '0' && raw[i] <= '9') i++;
                if (i == digits || i + 2 > raw.Length || raw[i] != ' ' || raw[i + 1] != ' ') continue;
                string name = raw.Substring(i + 2);
                if (name.Length > 0) names.Add(name);
            }
            return names;
        }

        // The names that contain every word of the filter, case-insensitive; names that start with the first
        // word come first. An empty filter keeps everything in the device's order.
        public static List<string> Filter(List<string> names, string filter)
        {
            string[] words = filter.ToLowerInvariant().Split(new char[] { ' ' }, StringSplitOptions.RemoveEmptyEntries);
            List<string> starts = new List<string>();
            List<string> others = new List<string>();
            foreach (string name in names)
            {
                string lower = name.ToLowerInvariant();
                bool all = true;
                foreach (string w in words) if (lower.IndexOf(w, StringComparison.Ordinal) < 0) { all = false; break; }
                if (!all) continue;
                if (words.Length > 0 && lower.StartsWith(words[0], StringComparison.Ordinal)) starts.Add(name); else others.Add(name);
            }
            starts.AddRange(others);
            return starts;
        }

        // One argument for a Windows command line (the rules of CommandLineToArgvW / the C runtime).
        public static string QuoteArgument(string arg)
        {
            if (arg.Length > 0 && arg.IndexOfAny(new char[] { ' ', '\t', '\n', '\v', '"' }) < 0) return arg;
            StringBuilder sb = new StringBuilder("\"");
            int backslashes = 0;
            foreach (char c in arg)
            {
                if (c == '\\') { backslashes++; continue; }
                if (c == '"') { sb.Append('\\', backslashes * 2 + 1); sb.Append('"'); }
                else { sb.Append('\\', backslashes); sb.Append(c); }
                backslashes = 0;
            }
            sb.Append('\\', backslashes * 2);
            sb.Append('"');
            return sb.ToString();
        }

        // "Ctrl+Alt+T" -> modifiers (MOD_ALT 1, MOD_CONTROL 2, MOD_SHIFT 4, MOD_WIN 8) and a key; false if invalid.
        public static bool ParseHotkey(string text, out uint modifiers, out Keys key)
        {
            modifiers = 0;
            key = Keys.None;
            if (string.IsNullOrEmpty(text)) return false;
            foreach (string part in text.Split('+'))
            {
                string p = part.Trim();
                switch (p.ToLowerInvariant())
                {
                    case "alt": modifiers |= 1; break;
                    case "ctrl": case "control": modifiers |= 2; break;
                    case "shift": modifiers |= 4; break;
                    case "win": modifiers |= 8; break;
                    default:
                        try { key = (Keys)Enum.Parse(typeof(Keys), p, true); }
                        catch (ArgumentException) { return false; }
                        break;
                }
            }
            // a hotkey needs a real key and at least one modifier, otherwise it would take over a plain key
            return key != Keys.None && modifiers != 0;
        }

        public static string FormatHotkey(Keys modifiers, Keys key)
        {
            StringBuilder sb = new StringBuilder();
            if ((modifiers & Keys.Control) != 0) sb.Append("Ctrl+");
            if ((modifiers & Keys.Alt) != 0) sb.Append("Alt+");
            if ((modifiers & Keys.Shift) != 0) sb.Append("Shift+");
            sb.Append(key.ToString());
            return sb.ToString();
        }

        // The last non-empty line of the output (trustctl prints the code alone on stdout).
        public static string LastLine(string output)
        {
            string[] lines = output.Replace("\r", "").Split('\n');
            for (int i = lines.Length - 1; i >= 0; i--)
                if (lines[i].Trim().Length > 0) return lines[i].Trim();
            return "";
        }

        // A TOTP code: 6 to 8 digits (leading zeros included).
        public static bool IsCode(string text)
        {
            if (text.Length < 6 || text.Length > 8) return false;
            foreach (char c in text) if (c < '0' || c > '9') return false;
            return true;
        }

        // Digits only in practice; escaped anyway so that SendKeys never reads a code as a command.
        public static string EscapeForSendKeys(string text)
        {
            StringBuilder sb = new StringBuilder();
            foreach (char c in text)
            {
                if ("+^%~(){}[]".IndexOf(c) >= 0) sb.Append('{').Append(c).Append('}');
                else sb.Append(c);
            }
            return sb.ToString();
        }
    }

    // ---------------------------------------------------------------- native
    static class Native
    {
        [DllImport("user32.dll")] public static extern bool RegisterHotKey(IntPtr hWnd, int id, uint modifiers, uint vk);
        [DllImport("user32.dll")] public static extern bool UnregisterHotKey(IntPtr hWnd, int id);
        [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
        [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
        [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hWnd, IntPtr processId);
        [DllImport("user32.dll")] public static extern bool AttachThreadInput(uint attach, uint to, bool attachIt);
        [DllImport("kernel32.dll")] public static extern uint GetCurrentThreadId();
        [DllImport("user32.dll")] public static extern bool DestroyIcon(IntPtr icon);

        // Windows only lets a process that has just had input take the foreground; borrowing the input queue of the
        // current foreground window is the usual way for a hotkey handler.
        public static void ForceForeground(IntPtr hWnd)
        {
            IntPtr current = GetForegroundWindow();
            uint other = GetWindowThreadProcessId(current, IntPtr.Zero);
            uint me = GetCurrentThreadId();
            bool attached = other != 0 && other != me && AttachThreadInput(me, other, true);
            SetForegroundWindow(hWnd);
            if (attached) AttachThreadInput(me, other, false);
        }
    }

    // ---------------------------------------------------------------- trustctl
    class TrustctlResult
    {
        public int ExitCode;
        public string Output = "";
        public string Error = "";
        public bool Started;
    }

    static class Trustctl
    {
        public static TrustctlResult Run(Settings s, string[] args, int timeoutMs)
        {
            TrustctlResult r = new TrustctlResult();
            StringBuilder line = new StringBuilder();
            if (s.Device.Length > 0) line.Append("--device ").Append(Logic.QuoteArgument(s.Device)).Append(' ');
            foreach (string a in args) line.Append(Logic.QuoteArgument(a)).Append(' ');
            ProcessStartInfo psi = new ProcessStartInfo(s.TrustctlPath, line.ToString().TrimEnd());
            psi.UseShellExecute = false;
            psi.CreateNoWindow = true;
            psi.RedirectStandardOutput = true;
            psi.RedirectStandardError = true;
            psi.RedirectStandardInput = true;
            psi.StandardOutputEncoding = Encoding.UTF8;
            psi.StandardErrorEncoding = Encoding.UTF8;
            Process p;
            try { p = Process.Start(psi); }
            catch (Exception e) { r.Error = "cannot start " + s.TrustctlPath + ": " + e.Message; return r; }
            r.Started = true;
            p.StandardInput.Close();  // trustctl never waits for input here
            StringBuilder err = new StringBuilder();
            p.ErrorDataReceived += delegate(object sender, DataReceivedEventArgs e) { if (e.Data != null) err.AppendLine(e.Data); };
            p.BeginErrorReadLine();
            string output = p.StandardOutput.ReadToEnd();
            if (!p.WaitForExit(timeoutMs))
            {
                try { p.Kill(); } catch (InvalidOperationException) { }
                r.Error = "trustctl did not finish in time";
                r.ExitCode = -1;
                return r;
            }
            p.WaitForExit();  // lets the asynchronous stderr reader finish
            r.Output = output;
            r.Error = err.ToString().Trim();
            r.ExitCode = p.ExitCode;
            return r;
        }
    }

    // ---------------------------------------------------------------- the picker (dmenu-like)
    class PickerForm : Form
    {
        readonly TextBox filter = new TextBox();
        readonly ListBox list = new ListBox();
        readonly List<string> all;
        public string Chosen;

        public PickerForm(List<string> names)
        {
            all = names;
            FormBorderStyle = FormBorderStyle.None;
            StartPosition = FormStartPosition.Manual;
            ShowInTaskbar = false;
            TopMost = true;
            KeyPreview = true;
            BackColor = SystemColors.WindowFrame;
            Padding = new Padding(1);
            Font = new Font("Segoe UI", 11f);

            filter.Dock = DockStyle.Top;
            filter.BorderStyle = BorderStyle.FixedSingle;
            list.Dock = DockStyle.Fill;
            list.BorderStyle = BorderStyle.None;
            list.IntegralHeight = false;
            Controls.Add(list);
            Controls.Add(filter);

            int rows = Math.Min(Math.Max(names.Count, 1), 10);
            ClientSize = new Size(420, filter.PreferredHeight + 2 + rows * list.ItemHeight);
            Screen screen = Screen.FromPoint(Cursor.Position);
            Location = new Point(screen.WorkingArea.Left + (screen.WorkingArea.Width - Width) / 2,
                                 screen.WorkingArea.Top + screen.WorkingArea.Height / 4);

            filter.TextChanged += delegate { Refill(); };
            filter.KeyDown += FilterKeyDown;
            list.MouseClick += delegate(object s, MouseEventArgs e)
            {
                int i = list.IndexFromPoint(e.Location);
                if (i >= 0) Choose(i);
            };
            Deactivate += delegate { if (Chosen == null && Visible) Close(); };
            Shown += delegate
            {
                Native.ForceForeground(Handle);
                Activate();
                filter.Focus();
            };
            Refill();
        }

        void Refill()
        {
            list.BeginUpdate();
            list.Items.Clear();
            foreach (string n in Logic.Filter(all, filter.Text)) list.Items.Add(n);
            if (list.Items.Count > 0) list.SelectedIndex = 0;
            list.EndUpdate();
        }

        void Choose(int index)
        {
            if (index < 0 || index >= list.Items.Count) return;
            Chosen = (string)list.Items[index];
            Close();
        }

        void FilterKeyDown(object sender, KeyEventArgs e)
        {
            switch (e.KeyCode)
            {
                case Keys.Escape: Close(); e.SuppressKeyPress = true; break;
                case Keys.Enter: Choose(list.SelectedIndex); e.SuppressKeyPress = true; break;
                case Keys.Down: MoveSelection(1); e.SuppressKeyPress = true; break;
                case Keys.Up: MoveSelection(-1); e.SuppressKeyPress = true; break;
                case Keys.PageDown: MoveSelection(9); e.SuppressKeyPress = true; break;
                case Keys.PageUp: MoveSelection(-9); e.SuppressKeyPress = true; break;
            }
        }

        void MoveSelection(int delta)
        {
            if (list.Items.Count == 0) return;
            list.SelectedIndex = Math.Max(0, Math.Min(list.Items.Count - 1, list.SelectedIndex + delta));
        }
    }

    // ---------------------------------------------------------------- settings dialog
    class SettingsForm : Form
    {
        readonly TextBox path = new TextBox();
        readonly TextBox device = new TextBox();
        readonly TextBox hotkey = new TextBox();
        readonly RadioButton type = new RadioButton();
        readonly RadioButton copy = new RadioButton();
        readonly CheckBox enter = new CheckBox();
        readonly NumericUpDown delay = new NumericUpDown();
        readonly CheckBox autostart = new CheckBox();
        public Settings Result;
        public bool AutostartWanted;

        const string RunKey = @"Software\Microsoft\Windows\CurrentVersion\Run";

        public static bool AutostartEnabled()
        {
            using (Microsoft.Win32.RegistryKey k = Microsoft.Win32.Registry.CurrentUser.OpenSubKey(RunKey))
                return k != null && k.GetValue("trustctl-tray") != null;
        }

        public static void SetAutostart(bool on)
        {
            using (Microsoft.Win32.RegistryKey k = Microsoft.Win32.Registry.CurrentUser.CreateSubKey(RunKey))
            {
                if (on) k.SetValue("trustctl-tray", "\"" + Application.ExecutablePath + "\"");
                else if (k.GetValue("trustctl-tray") != null) k.DeleteValue("trustctl-tray");
            }
        }

        Label Caption(string text, int y)
        {
            Label l = new Label();
            l.Text = text;
            l.AutoSize = true;
            l.Location = new Point(12, y + 4);
            Controls.Add(l);
            return l;
        }

        // A note under a field; it wraps inside the window instead of running out of it.
        void Hint(string text, int x, int y)
        {
            Label l = new Label();
            l.Text = text;
            l.AutoSize = true;
            l.MaximumSize = new Size(508 - x, 0);
            l.ForeColor = SystemColors.GrayText;
            l.Location = new Point(x, y);
            Controls.Add(l);
        }

        public SettingsForm(Settings s)
        {
            AutoScaleDimensions = new SizeF(96f, 96f);  // follow the display scaling (125 %, 150 %) so text fits
            AutoScaleMode = AutoScaleMode.Dpi;
            Text = "trustctl-tray settings";
            FormBorderStyle = FormBorderStyle.FixedDialog;
            MaximizeBox = false;
            MinimizeBox = false;
            StartPosition = FormStartPosition.CenterScreen;
            ClientSize = new Size(520, 392);
            Font = new Font("Segoe UI", 9f);

            Caption("Path of trustctl.exe", 12);
            path.SetBounds(150, 12, 280, 24);
            path.Text = s.TrustctlPath;
            Button browse = new Button();
            browse.Text = "Browse...";
            browse.SetBounds(438, 11, 70, 26);
            browse.Click += delegate
            {
                OpenFileDialog d = new OpenFileDialog();
                d.Filter = "trustctl.exe|trustctl*.exe|Programs|*.exe";
                if (d.ShowDialog(this) == DialogResult.OK) path.Text = d.FileName;
            };

            Caption("Device (optional)", 48);
            device.SetBounds(150, 48, 120, 24);
            device.Text = s.Device;
            Hint("A port such as COM5; empty uses trustctl's own setting.", 150, 74);

            Caption("Hotkey (optional)", 108);
            hotkey.SetBounds(150, 108, 160, 24);
            hotkey.ReadOnly = true;
            hotkey.Text = s.Hotkey;
            hotkey.KeyDown += HotkeyKeyDown;
            Hint("Click the field, then press the keys (a modifier is needed). Backspace clears it.", 150, 134);

            GroupBox what = new GroupBox();
            what.Text = "When the device has given the code";
            what.SetBounds(12, 180, 496, 130);
            type.Text = "Type it into the window that was active before";
            type.Location = new Point(12, 22);
            type.AutoSize = true;
            type.Checked = s.Action == "type";
            // what belongs to typing hangs under it: the Enter key and the wait before the first key
            enter.Text = "Then press Enter";
            enter.Location = new Point(32, 46);
            enter.AutoSize = true;
            enter.Checked = s.PressEnter;
            Label wait = new Label();
            wait.Text = "Wait";
            wait.AutoSize = true;
            wait.Location = new Point(32, 76);
            delay.SetBounds(72, 72, 64, 24);
            delay.Minimum = 0;
            delay.Maximum = 10000;
            delay.Increment = 100;
            delay.Value = s.DelayMs;
            Label ms = new Label();
            ms.Text = "ms before typing (time to let go of the hotkey)";
            ms.AutoSize = true;
            ms.Location = new Point(142, 76);
            copy.Text = "Copy it to the clipboard";
            copy.Location = new Point(12, 102);
            copy.AutoSize = true;
            copy.Checked = s.Action == "copy";
            what.Controls.AddRange(new Control[] { type, enter, wait, delay, ms, copy });
            type.CheckedChanged += delegate { enter.Enabled = delay.Enabled = type.Checked; };
            enter.Enabled = delay.Enabled = type.Checked;

            autostart.Text = "Start with Windows";
            autostart.Location = new Point(12, 324);
            autostart.AutoSize = true;
            autostart.Checked = AutostartEnabled();

            Button ok = new Button();
            ok.Text = "OK";
            ok.SetBounds(352, 352, 75, 28);
            ok.Click += delegate { Accept(); };
            Button cancel = new Button();
            cancel.Text = "Cancel";
            cancel.SetBounds(433, 352, 75, 28);
            cancel.DialogResult = DialogResult.Cancel;
            AcceptButton = ok;
            CancelButton = cancel;
            Controls.AddRange(new Control[] { path, browse, device, hotkey, what, autostart, ok, cancel });
        }

        void HotkeyKeyDown(object sender, KeyEventArgs e)
        {
            e.SuppressKeyPress = true;
            e.Handled = true;
            if (e.KeyCode == Keys.Back || e.KeyCode == Keys.Delete) { hotkey.Text = ""; return; }
            if (e.KeyCode == Keys.ControlKey || e.KeyCode == Keys.ShiftKey || e.KeyCode == Keys.Menu ||
                e.KeyCode == Keys.LWin || e.KeyCode == Keys.RWin) return;  // wait for the real key
            Keys mods = e.Modifiers & (Keys.Control | Keys.Alt | Keys.Shift);
            if (mods == 0) return;  // a hotkey without a modifier would take a plain key away from every program
            hotkey.Text = Logic.FormatHotkey(mods, e.KeyCode);
        }

        void Accept()
        {
            Settings s = new Settings();
            s.TrustctlPath = path.Text.Trim().Length > 0 ? path.Text.Trim() : "trustctl.exe";
            s.Device = device.Text.Trim();
            s.Hotkey = hotkey.Text;
            s.Action = copy.Checked ? "copy" : "type";
            s.PressEnter = enter.Checked;
            s.DelayMs = (int)delay.Value;
            Result = s;
            AutostartWanted = autostart.Checked;
            DialogResult = DialogResult.OK;
            Close();
        }
    }

    // ---------------------------------------------------------------- the application (tray icon, hotkey)
    class TrayContext : ApplicationContext
    {
        const int HotkeyId = 1;
        const int WM_HOTKEY = 0x0312;

        readonly NotifyIcon icon = new NotifyIcon();
        readonly HotkeyWindow window;
        readonly Control ui = new Control();  // never shown: carries results of the worker thread back to the UI thread
        Settings settings;
        bool busy;

        class HotkeyWindow : NativeWindow
        {
            public Action Pressed;
            public HotkeyWindow() { CreateHandle(new CreateParams()); }
            protected override void WndProc(ref Message m)
            {
                if (m.Msg == WM_HOTKEY && m.WParam.ToInt32() == HotkeyId && Pressed != null) Pressed();
                base.WndProc(ref m);
            }
        }

        public TrayContext()
        {
            IntPtr unused = ui.Handle;  // creates the handle on the UI thread (Application.Run has not started yet here)
            settings = Settings.Load(Settings.FilePath);
            window = new HotkeyWindow();
            window.Pressed = delegate { ShowPicker(); };

            ContextMenuStrip menu = new ContextMenuStrip();
            menu.Items.Add("Get a code...", null, delegate { ShowPicker(); });
            menu.Items.Add("Settings...", null, delegate { ShowSettings(); });
            menu.Items.Add(new ToolStripSeparator());
            menu.Items.Add("Exit", null, delegate { Quit(); });
            icon.ContextMenuStrip = menu;
            icon.Icon = MakeIcon();
            icon.Text = "trustctl";
            icon.MouseClick += delegate(object s, MouseEventArgs e) { if (e.Button == MouseButtons.Left) ShowPicker(); };
            icon.Visible = true;

            RegisterHotkey();
            if (!File.Exists(Settings.FilePath)) ShowSettings();  // first start: ask for the path of trustctl
        }

        static Icon MakeIcon()
        {
            using (Bitmap b = new Bitmap(32, 32))
            {
                using (Graphics g = Graphics.FromImage(b))
                {
                    g.Clear(Color.Transparent);
                    g.FillEllipse(Brushes.SteelBlue, 1, 1, 30, 30);
                    using (Font f = new Font("Segoe UI", 15f, FontStyle.Bold, GraphicsUnit.Pixel))
                        g.DrawString("T", f, Brushes.White, 8, 5);
                }
                IntPtr h = b.GetHicon();
                Icon copy = (Icon)Icon.FromHandle(h).Clone();
                Native.DestroyIcon(h);
                return copy;
            }
        }

        void RegisterHotkey()
        {
            Native.UnregisterHotKey(window.Handle, HotkeyId);
            if (settings.Hotkey.Length == 0) return;
            uint mods;
            Keys key;
            if (!Logic.ParseHotkey(settings.Hotkey, out mods, out key))
            {
                icon.ShowBalloonTip(5000, "trustctl", "The hotkey \"" + settings.Hotkey + "\" is not valid.", ToolTipIcon.Warning);
                return;
            }
            if (!Native.RegisterHotKey(window.Handle, HotkeyId, mods, (uint)key))
                icon.ShowBalloonTip(5000, "trustctl", "The hotkey " + settings.Hotkey + " is taken by another program.", ToolTipIcon.Warning);
        }

        void ShowSettings()
        {
            using (SettingsForm f = new SettingsForm(settings))
            {
                if (f.ShowDialog() != DialogResult.OK) return;
                settings = f.Result;
                try
                {
                    settings.Save(Settings.FilePath);
                    SettingsForm.SetAutostart(f.AutostartWanted);
                }
                catch (Exception e) { Error("Could not save the settings: " + e.Message); }
                RegisterHotkey();
            }
        }

        void Error(string text)
        {
            icon.ShowBalloonTip(8000, "trustctl", text, ToolTipIcon.Error);
        }

        void ShowPicker()
        {
            if (busy) return;  // the device is waiting for a button press
            IntPtr previous = Native.GetForegroundWindow();
            busy = true;
            TrustctlResult list;
            try { list = Trustctl.Run(settings, new string[] { "list" }, 20000); }
            finally { busy = false; }
            if (!list.Started || list.ExitCode != 0)
            {
                Error(list.Error.Length > 0 ? list.Error : "trustctl list failed (exit " + list.ExitCode + ")");
                return;
            }
            List<string> names = Logic.ParseList(list.Output);
            if (names.Count == 0)
            {
                Error("The device holds no keys.");
                return;
            }
            string chosen;
            using (PickerForm picker = new PickerForm(names))
            {
                picker.ShowDialog();
                chosen = picker.Chosen;
            }
            if (chosen == null) return;
            GetToken(chosen, previous);
        }

        void GetToken(string name, IntPtr previous)
        {
            busy = true;
            icon.ShowBalloonTip(30000, "trustctl", "Hold the button on the device for 1 to 2 seconds for " + name + ", then let go.", ToolTipIcon.Info);
            // the device waits up to 30 s for the button; do not block the message loop meanwhile
            ThreadPool.QueueUserWorkItem(delegate
            {
                TrustctlResult r;
                try { r = Trustctl.Run(settings, new string[] { "token", name }, 60000); }
                catch (Exception e)
                {
                    r = new TrustctlResult();
                    r.ExitCode = -1;
                    r.Error = "trustctl token failed: " + e.Message;
                }
                ui.BeginInvoke((MethodInvoker)delegate { Deliver(r, name, previous); });
            });
        }

        void Deliver(TrustctlResult r, string name, IntPtr previous)
        {
            busy = false;
            icon.Visible = false;  // the only way to withdraw the balloon early
            icon.Visible = true;
            string code = Logic.LastLine(r.Output);
            if (r.ExitCode == 0 && !Logic.IsCode(code))
            {
                Error("trustctl printed something unexpected instead of a code: " + (r.Output.Trim().Length > 60 ? r.Output.Trim().Substring(0, 60) + "..." : r.Output.Trim()));
                return;
            }
            if (r.ExitCode != 0 || code.Length == 0)
            {
                string why = r.Error.Length > 0 ? r.Error : "trustctl token failed (exit " + r.ExitCode + ")";
                if (r.ExitCode == 2) why = "Not confirmed on the device (denied or timed out).";
                Error(why);
                return;
            }
            if (settings.Action == "copy")
            {
                try { Clipboard.SetText(code); }
                catch (Exception e) { Error("Could not use the clipboard: " + e.Message); return; }
                icon.ShowBalloonTip(2000, "trustctl", "The code for " + name + " is in the clipboard.", ToolTipIcon.Info);
                return;
            }
            try
            {
                if (previous != IntPtr.Zero) Native.ForceForeground(previous);
                Thread.Sleep(Math.Max(settings.DelayMs, 50));
                SendKeys.SendWait(Logic.EscapeForSendKeys(code) + (settings.PressEnter ? "{ENTER}" : ""));
            }
            catch (Exception e) { Error("Could not type the code: " + e.Message); }
        }

        void Quit()
        {
            Native.UnregisterHotKey(window.Handle, HotkeyId);
            icon.Visible = false;
            icon.Dispose();
            ExitThread();
        }
    }

    static class Program
    {
        [STAThread]
        static void Main(string[] args)
        {
            bool created;
            using (Mutex single = new Mutex(true, "trustctl-tray-single-instance", out created))
            {
                if (!created) return;  // already running: its icon is in the tray
                Application.EnableVisualStyles();
                Application.SetCompatibleTextRenderingDefault(false);
                Application.Run(new TrayContext());
            }
        }
    }
}
