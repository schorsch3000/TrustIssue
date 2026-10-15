package hexfile_test

import (
	"strings"
	"testing"

	"trustissues/trustctl/internal/hexfile"
)

func TestParseAndPages(t *testing.T) {
	// 4 bytes at 0x0000 and 2 bytes at 0x0080 (the second page), then the end record
	hex := ":0400000001020304F2\n:02008000AABB19\n:00000001FF\n"
	img, err := hexfile.Parse(strings.NewReader(hex))
	if err != nil {
		t.Fatal(err)
	}
	if img.Data[2] != 3 || img.Data[0x81] != 0xBB || img.Data[4] != 0xFF || img.Highest() != 0x81 {
		t.Fatalf("image: % x ... highest %d", img.Data[:6], img.Highest())
	}
	pages := img.Pages(128)
	if len(pages) != 2 || pages[0].Address != 0 || pages[1].Address != 128 || len(pages[0].Data) != 128 || pages[1].Data[3] != 0xFF {
		t.Fatalf("pages: %+v", pages)
	}
}

func TestParseErrors(t *testing.T) {
	for name, hex := range map[string]string{
		"checksum":     ":0400000001020304F3\n",
		"no colon":     "0400000001020304F2\n",
		"too short":    ":0400000001\n",
		"beyond flash": ":02FFFF00AABB98\n:00000001FF\n",
	} {
		if _, err := hexfile.Parse(strings.NewReader(hex)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestExtendedAddress(t *testing.T) {
	// extended segment address 0x0010 (a base of 0x100), then one byte at 0x0002
	hex := ":020000020010EC\n:0100020055A8\n:00000001FF\n"
	img, err := hexfile.Parse(strings.NewReader(hex))
	if err != nil || img.Data[0x102] != 0x55 {
		t.Fatalf("%v %x", err, img.Data[0x100:0x104])
	}
}
