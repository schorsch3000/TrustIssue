// Package profile keeps the per-device settings that trustctl patches into the
// firmware when it flashes: contrast values, a logo and a font.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"trustissues/trustctl/internal/fonts"
	"trustissues/trustctl/internal/logo"
	"trustissues/trustctl/internal/params"
)

// Profile is stored as ~/.config/trustctl/device.json. Paths of the logo and
// the font should be absolute; a built-in font is written "builtin:NAME".
type Profile struct {
	ContrastFull *int   `json:"contrast_full,omitempty"`
	ContrastDim  *int   `json:"contrast_dim,omitempty"`
	Logo         string `json:"logo,omitempty"`
	LogoInvert   bool   `json:"logo_invert,omitempty"`
	Font         string `json:"font,omitempty"`
}

// DefaultPath is where the profile lives.
func DefaultPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "device.json"
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "trustctl", "device.json")
}

// Load reads a profile; a missing file is an empty profile.
func Load(path string) (Profile, error) {
	var p Profile
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Save writes the profile.
func (p Profile) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func contrast(v *int) (*uint8, error) {
	if v == nil {
		return nil, nil
	}
	if *v < 0 || *v > 255 {
		return nil, fmt.Errorf("a contrast value must be between 0 and 255, not %d", *v)
	}
	b := uint8(*v)
	return &b, nil
}

// Patch reads the logo and the font files the profile names and returns what
// to patch into the firmware.
func (p Profile) Patch() (params.Patch, error) {
	var patch params.Patch
	var err error
	if patch.ContrastFull, err = contrast(p.ContrastFull); err != nil {
		return patch, err
	}
	if patch.ContrastDim, err = contrast(p.ContrastDim); err != nil {
		return patch, err
	}
	if p.Logo != "" {
		f, err := os.Open(p.Logo)
		if err != nil {
			return patch, fmt.Errorf("logo: %w", err)
		}
		defer f.Close()
		if patch.Logo, err = logo.Convert(f, p.LogoInvert); err != nil {
			return patch, fmt.Errorf("logo %s: %w", p.Logo, err)
		}
	}
	if p.Font != "" {
		if patch.Font, err = fonts.Load(p.Font); err != nil {
			return patch, fmt.Errorf("font: %w", err)
		}
		if err := params.ValidateFont(patch.Font); err != nil {
			return patch, fmt.Errorf("font %s: %w", p.Font, err)
		}
	}
	return patch, nil
}
