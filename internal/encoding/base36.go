package encoding

import (
	"fmt"
	"strconv"
	"strings"
)

// EncodeBase36 encodes an integer to a base36 string (uppercase).
func EncodeBase36(value int) string {
	return strings.ToUpper(strconv.FormatInt(int64(value), 36))
}

// EncodeBase36Pad encodes an integer to base36, zero-padded to minWidth.
func EncodeBase36Pad(value, minWidth int) string {
	s := EncodeBase36(value)
	for len(s) < minWidth {
		s = "0" + s
	}
	return s
}

// DecodeBase36 decodes a base36 string to an integer.
func DecodeBase36(s string) (int, error) {
	v, err := strconv.ParseInt(strings.ToLower(s), 36, 64)
	if err != nil {
		return 0, fmt.Errorf("decoding base36 %q: %w", s, err)
	}
	return int(v), nil
}
