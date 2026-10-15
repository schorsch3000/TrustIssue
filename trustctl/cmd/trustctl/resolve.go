package main

import (
	"fmt"
	"strconv"
	"strings"

	"trustissues/trustctl/internal/protocol"
)

// resolve maps what the user typed to a key: an exact name (any case), else a
// slot number, else a unique prefix of a name.
func resolve(keys []protocol.Key, arg string) (protocol.Key, error) {
	for _, k := range keys {
		if strings.EqualFold(k.Name, arg) {
			return k, nil
		}
	}
	if id, err := strconv.Atoi(arg); err == nil {
		for _, k := range keys {
			if k.ID == id {
				return k, nil
			}
		}
	}
	var matches []protocol.Key
	for _, k := range keys {
		if strings.HasPrefix(strings.ToLower(k.Name), strings.ToLower(arg)) {
			matches = append(matches, k)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return protocol.Key{}, fmt.Errorf("no key %q on the device (see: trustctl list)", arg)
	}
	names := make([]string, len(matches))
	for i, k := range matches {
		names[i] = fmt.Sprintf("%q", k.Name)
	}
	return protocol.Key{}, fmt.Errorf("%q matches several keys: %s", arg, strings.Join(names, ", "))
}
