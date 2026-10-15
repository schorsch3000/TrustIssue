package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryCommandIsInEveryScript(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		script := completionScript(shell)
		for _, c := range commandTable {
			if !strings.Contains(script, c.name) {
				t.Errorf("%s completion lacks %q", shell, c.name)
			}
		}
		for _, word := range []string{"device", "autoprobe", "simulate", "no-daemon", "__keys", "calibrate", "contrast"} { // fish writes options as "-l device"
			if !strings.Contains(script, word) {
				t.Errorf("%s completion lacks %q", shell, word)
			}
		}
	}
}

func TestEveryDispatchedCommandHasACompletion(t *testing.T) {
	// the commands that the usage text lists must all be in the table
	names := commandNames()
	for _, line := range strings.Split(usage, "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			word := strings.Fields(line)[0]
			if word != "--device" && word != "--autoprobe" && word != "--simulate" && word != "--no-daemon" &&
				!strings.Contains(" "+names+" ", " "+word+" ") {
				t.Errorf("%q is in the usage text but not in the completion table", word)
			}
		}
	}
}

func TestAutocompleteCommands(t *testing.T) {
	for command, marker := range map[string]string{
		"autocomplete":      "complete -F _trustctl trustctl",
		"autocomplete-sh":   "complete -F _trustctl trustctl",
		"autocomplete-zsh":  "#compdef trustctl",
		"autocomplete-fish": "complete -c trustctl",
	} {
		var out, errOut bytes.Buffer
		if code := run([]string{command}, &out, &errOut); code != exitOK || !strings.Contains(out.String(), marker) {
			t.Errorf("%s: exit %d, output starts %q", command, code, out.String()[:min(60, out.Len())])
		}
	}
}

// bashComplete runs the generated bash completion in a real bash for a command line
// (the word under the cursor last) and returns the offered words. A fake trustctl
// in the PATH answers __keys.
func bashComplete(t *testing.T, words ...string) []string {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	dir := t.TempDir()
	fake := "#!/bin/sh\n[ \"$1\" = __keys ] && printf 'GitHub\\nGitLab work\\nAWS\\n'\n[ \"$1\" = font ] && printf '5x8\\nvictoriabold8\\nvictoriamedium8  (the firmware default)\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "trustctl"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"firmware.hex", "other.txt", "logo.png", "font.bin", "profile.json"} {
		os.WriteFile(filepath.Join(dir, name), nil, 0o644)
	}
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	script := completionScript("bash") + "\n"
	quoted := make([]string, len(words))
	for i, w := range words {
		quoted[i] = "'" + w + "'"
	}
	script += "COMP_WORDS=(trustctl " + strings.Join(quoted, " ") + ")\nCOMP_CWORD=" + string(rune('0'+len(words))) + "\n_trustctl\nprintf '%s\\n' \"${COMPREPLY[@]}\"\n"
	cmd := exec.Command(bash, "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	var got []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			got = append(got, l)
		}
	}
	return got
}

func same(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, x := range a {
		seen[x] = true
	}
	for _, x := range b {
		if !seen[x] {
			return false
		}
	}
	return true
}

func TestBashCompletion(t *testing.T) {
	cases := []struct {
		line []string
		want []string
	}{
		{[]string{"to"}, []string{"token"}},
		{[]string{"a"}, []string{"add", "autocomplete", "autocomplete-zsh", "autocomplete-fish"}},
		{[]string{"autocomplete-"}, []string{"autocomplete-zsh", "autocomplete-fish"}},
		{[]string{"--"}, []string{"--device", "--autoprobe", "--simulate", "--no-daemon"}},
		{[]string{"--device", "/dev/null", "to"}, []string{"token"}},
		{[]string{"token", ""}, []string{"GitHub", "GitLab\\ work", "AWS"}},
		{[]string{"token", "git"}, []string{"GitHub", "GitLab\\ work"}},
		{[]string{"del", "aw"}, []string{"AWS"}},
		{[]string{"token", "AWS", ""}, nil},
		{[]string{"--device", "x", "token", "gitl"}, []string{"GitLab\\ work"}},
		{[]string{"tz", ""}, []string{"get", "sync", "set"}},
		{[]string{"config", ""}, []string{"get", "set"}},
		{[]string{"config", "set", "rot"}, []string{"rotate=on", "rotate=off"}},
		{[]string{"daemon", "s"}, []string{"status", "stop"}},
		{[]string{"add", "--"}, []string{"--digits", "--period"}},
		{[]string{"flash", ""}, []string{"core", "calibration", "firmware.hex", "sub"}},
		{[]string{"--device", "/dev/ttyUSB0", "flash", ""}, []string{"core", "calibration", "firmware.hex", "sub"}},
		{[]string{"flash", "ca"}, []string{"calibration"}},
		{[]string{"flash", "--"}, []string{"--no-patch", "--profile", "--font", "--wait", "--countdown"}},
		{[]string{"flash", "--font", "5"}, []string{"5x8"}},
		{[]string{"flash", "--profile", ""}, []string{"profile.json", "sub"}},
		{[]string{"font", ""}, []string{"list", "extract"}},
		{[]string{"logo", ""}, []string{"extract"}},
		{[]string{"font", "extract", "--"}, []string{"--from", "--profile", "--name"}},
		{[]string{"logo", "extract", ""}, []string{"logo.png", "sub"}},
		{[]string{"logo", "extract", "--"}, []string{"--from", "--profile"}},
		{[]string{"calibrate", ""}, []string{"contrast", "logo", "font", "show", "forget"}},
		{[]string{"calibrate", "contrast", ""}, []string{"full", "dim"}},
		{[]string{"calibrate", "logo", ""}, []string{"logo.png", "sub"}},
		{[]string{"calibrate", "font", ""}, []string{"5x8", "victoriabold8", "victoriamedium8", "font.bin", "sub"}},
		{[]string{"calibrate", "forget", ""}, []string{"logo", "font", "contrast"}},
	}
	for _, c := range cases {
		if got := bashComplete(t, c.line...); !same(got, c.want...) {
			t.Errorf("%q: got %q, want %q", c.line, got, c.want)
		}
	}
}
