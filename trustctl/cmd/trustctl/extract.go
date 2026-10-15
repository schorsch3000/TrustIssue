package main

import (
	"fmt"
	"image/png"
	"io"
	"os"

	"trustissues/trustctl/internal/hexfile"
	"trustissues/trustctl/internal/logo"
	"trustissues/trustctl/internal/params"
	"trustissues/trustctl/internal/profile"
)

const extractUsage = `  font extract FILE.bin [--from FILE.hex]   write the current font (772 bytes, U8x8 format) to work on
  logo extract FILE.png [--from FILE.hex]   write the current logo (128x32 PNG, white = lit) to work on
        --profile PATH
`

// extractCommand implements "trustctl font extract" and "trustctl logo extract".
// The device cannot be read back (flash lock bits), so the current blob is what
// "flash" would patch in: the profile's font or logo, or with --from the one
// stored in a firmware hex file. Without a profile font the built-in font is
// written; without a logo there is nothing to extract.
func extractCommand(kind string, args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 || args[0] != "extract" {
		return usageError("usage:\n%s", extractUsage)
	}
	target := args[1]
	profilePath, from := profile.DefaultPath(), ""
	for i := 2; i < len(args); i++ {
		switch {
		case args[i] == "--profile" && i+1 < len(args):
			profilePath = args[i+1]
			i++
		case args[i] == "--from" && i+1 < len(args):
			from = args[i+1]
			i++
		default:
			return usageError("%s extract: unknown option %q\n%s", kind, args[i], extractUsage)
		}
	}

	var data []byte
	var source string
	if from != "" {
		f, err := os.Open(from)
		if err != nil {
			return err
		}
		img, err := hexfile.Parse(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", from, err)
		}
		font, bitmap, err := params.Extract(img)
		if err != nil {
			return fmt.Errorf("%s: %w", from, err)
		}
		if kind == "font" {
			data, source = font, "the font in "+from
			if data == nil {
				data, source = params.DefaultFont, "the built-in font (the firmware in "+from+" does not carry its own)"
			}
		} else {
			data, source = bitmap, "the logo in "+from
		}
	} else {
		p, err := profile.Load(profilePath)
		if err != nil {
			return err
		}
		if kind == "font" {
			data, source = params.DefaultFont, "the built-in font (no font in the profile)"
			if p.Font != "" {
				patch, err := p.Patch()
				if err != nil {
					return err
				}
				data, source = patch.Font, "the profile's font "+p.Font
			}
		} else if p.Logo != "" {
			patch, err := p.Patch()
			if err != nil {
				return err
			}
			data, source = patch.Logo, "the profile's logo "+p.Logo
			if p.LogoInvert {
				source += " (inverted, as it appears on the display)"
			}
		}
	}
	if data == nil {
		return fmt.Errorf("there is no logo to extract: the profile %s names none and the firmware default has none; start from a new 128x32 image", profilePath)
	}

	if kind == "font" {
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	} else {
		img, err := logo.ToImage(data)
		if err != nil {
			return err
		}
		f, err := os.Create(target)
		if err != nil {
			return err
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	fmt.Fprintf(stderr, "Wrote %s from %s.\n", target, source)
	return nil
}
