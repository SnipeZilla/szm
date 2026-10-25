package proc

import (
	"fmt"
	"strings"
	"unicode/utf16"
)

// envBlock: the environment of a server for CreateProcess (Windows): each NAME=value ended by a NUL, one more NUL
// after the last. Its own variables replace those of the manager of the same name (Windows names ignore case).
func envBlock(base, own []string) ([]uint16, error) {
	mine := map[string]bool{}
	for _, kv := range own {
		mine[envName(kv)] = true
	}
	var out []uint16
	add := func(kv string) error {
		if strings.IndexByte(kv, 0) >= 0 {
			return fmt.Errorf("environment variable %s: a NUL in it", envName(kv))
		}
		out = append(out, utf16.Encode([]rune(kv))...)
		out = append(out, 0)
		return nil
	}
	for _, kv := range base {
		if kv == "" || mine[envName(kv)] {
			continue
		}
		if err := add(kv); err != nil {
			return nil, err
		}
	}
	for _, kv := range own {
		if err := add(kv); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		out = append(out, 0)
	}
	return append(out, 0), nil
}

// envName: the name of NAME=value, upper case ("=C:" for the "=C:=C:\\" Windows keeps per drive).
func envName(kv string) string {
	k, _, _ := strings.Cut(kv[min(1, len(kv)):], "=")
	return strings.ToUpper(kv[:min(1, len(kv))] + k)
}
