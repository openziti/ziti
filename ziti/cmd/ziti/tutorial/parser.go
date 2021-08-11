package tutorial

import (
	"strings"
	"unicode"
)

func ParseArgumentsWithStrings(val string) []string {
	var result []string

	current := &strings.Builder{}
	inString := false
	for _, r := range val {
		if r == '\'' {
			if inString {
				inString = false
			} else {
				inString = true
			}
		} else if !inString && unicode.IsSpace(r) {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		} else {
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}
