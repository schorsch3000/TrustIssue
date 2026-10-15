// Package fonts carries the 8x8 fonts that are built into trustctl (see README.md
// in this directory for their sources and licenses).
package fonts

import (
	"embed"
	"fmt"
	"os"
	"sort"
	"strings"
)

//go:embed *.bin
var files embed.FS

// Default is the font the firmware ships with.
const Default = "victoriamedium8"

// Prefix marks a built-in font in a profile ("builtin:victoriabold8").
const Prefix = "builtin:"

// Names lists the built-in fonts, sorted.
func Names() []string {
	entries, _ := files.ReadDir(".")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".bin"))
	}
	sort.Strings(names)
	return names
}

// Get returns the 772 bytes of a built-in font.
func Get(name string) ([]byte, bool) {
	data, err := files.ReadFile(strings.TrimPrefix(name, Prefix) + ".bin")
	return data, err == nil
}

// Builtin reports whether spec names a built-in font: either "builtin:NAME", or
// a bare NAME that is no path (no directory part, no extension).
func Builtin(spec string) bool {
	if strings.HasPrefix(spec, Prefix) {
		return true
	}
	if strings.ContainsAny(spec, "/\\.") {
		return false
	}
	_, ok := Get(spec)
	return ok
}

// Load returns the font data for a spec: a built-in name or the path of a font file.
func Load(spec string) ([]byte, error) {
	if Builtin(spec) {
		data, ok := Get(spec)
		if !ok {
			return nil, fmt.Errorf("there is no built-in font %q (try: trustctl font list)", strings.TrimPrefix(spec, Prefix))
		}
		return data, nil
	}
	return os.ReadFile(spec)
}

// Canonical is what a profile stores for spec: "builtin:NAME" for a built-in
// font, else the absolute path made by abs.
func Canonical(spec string, abs func(string) (string, error)) (string, error) {
	if Builtin(spec) {
		return Prefix + strings.TrimPrefix(spec, Prefix), nil
	}
	return abs(spec)
}
