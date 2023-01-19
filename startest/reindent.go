package startest

import (
	"fmt"
	"regexp"
	"strings"
)

func Reindent(in string) (string, error) {
	lines := regexp.MustCompile("\r\n|\r|\n").Split(in, -1)
	if len(lines) <= 1 {
		return in, nil
	}

	sb := strings.Builder{}
	sb.Grow(len(in))

	var trim string
	var trimSet bool
	for i, line := range lines {
		if !trimSet {
			trimmed := strings.TrimLeft(line, " \t")
			if trimmed == "" {
				if i != 0 {
					sb.WriteRune('\n')
				}
				continue
			}
			trim = line[:len(line)-len(trimmed)]
			trimSet = true
		}
		trimmed := strings.TrimPrefix(line, trim)
		if len(trimmed) == len(line) && trim != "" && strings.Trim(line, " \t") != "" {
			return "", fmt.Errorf("Invalid indentation on line %d: expected line starting %#v but got %#v", i+1, trim, line)
		}
		sb.WriteString(trimmed)
		sb.WriteRune('\n')
	}

	return sb.String(), nil
}
