package main

import (
	_ "embed"
	"fmt"
	"io"
	"math/rand"
	"strings"
)

//go:embed helo_messages.txt
var heloMessages string

// heloReply answers the misspelled "helo" with one of the canned lines.
func heloReply(stderr io.Writer) int {
	var lines []string
	for _, l := range strings.Split(heloMessages, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > 0 {
		fmt.Fprintln(stderr, lines[rand.Intn(len(lines))])
	}
	return exitUsage
}
