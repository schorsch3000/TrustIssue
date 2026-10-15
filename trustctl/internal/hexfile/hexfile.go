// Package hexfile reads Intel HEX files into a flash image.
package hexfile

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// FlashSize is the size of the ATmega328P's flash.
const FlashSize = 32768

// Image is a sparse flash image: Set tells which bytes the file defined.
type Image struct {
	Data [FlashSize]byte
	Set  [FlashSize]bool
}

// Parse reads an Intel HEX file (data, end of file and extended address
// records; the chip has only 32 KB, so addresses beyond are an error).
func Parse(r io.Reader) (*Image, error) {
	img := &Image{}
	for i := range img.Data {
		img.Data[i] = 0xFF
	}
	var base uint32
	scanner := bufio.NewScanner(r)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line[0] != ':' {
			return nil, fmt.Errorf("line %d: not an Intel HEX record", lineNumber)
		}
		raw, err := hex.DecodeString(line[1:])
		if err != nil || len(raw) < 5 || len(raw) != int(raw[0])+5 {
			return nil, fmt.Errorf("line %d: malformed record", lineNumber)
		}
		var sum byte
		for _, b := range raw {
			sum += b
		}
		if sum != 0 {
			return nil, fmt.Errorf("line %d: bad checksum", lineNumber)
		}
		length, address, kind, payload := int(raw[0]), uint32(raw[1])<<8|uint32(raw[2]), raw[3], raw[4:4+int(raw[0])]
		switch kind {
		case 0x00:
			for i := 0; i < length; i++ {
				at := base + address + uint32(i)
				if at >= FlashSize {
					return nil, fmt.Errorf("line %d: address %#x is beyond the 32 KB of flash", lineNumber, at)
				}
				img.Data[at] = payload[i]
				img.Set[at] = true
			}
		case 0x01:
			return img, nil
		case 0x02:
			base = (uint32(payload[0])<<8 | uint32(payload[1])) << 4
		case 0x04:
			base = (uint32(payload[0])<<8 | uint32(payload[1])) << 16
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return img, nil
}

// Page is one flash page of the image.
type Page struct {
	Address int
	Data    []byte
}

// Pages returns every page that has at least one defined byte; undefined
// bytes in it are 0xFF.
func (img *Image) Pages(size int) []Page {
	var pages []Page
	for start := 0; start < FlashSize; start += size {
		used := false
		for i := start; i < start+size; i++ {
			used = used || img.Set[i]
		}
		if used {
			pages = append(pages, Page{Address: start, Data: append([]byte(nil), img.Data[start:start+size]...)})
		}
	}
	return pages
}

// Write defines bytes of the image.
func (img *Image) Write(address int, data []byte) error {
	if address < 0 || address+len(data) > FlashSize {
		return fmt.Errorf("writing %d bytes at %#x leaves the flash", len(data), address)
	}
	for i, b := range data {
		img.Data[address+i] = b
		img.Set[address+i] = true
	}
	return nil
}

// Bytes returns n bytes from address.
func (img *Image) Bytes(address, n int) []byte {
	return append([]byte(nil), img.Data[address:address+n]...)
}

// Highest returns the highest defined address, or -1.
func (img *Image) Highest() int {
	for i := FlashSize - 1; i >= 0; i-- {
		if img.Set[i] {
			return i
		}
	}
	return -1
}
