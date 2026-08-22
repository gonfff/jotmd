package notes

import (
	"fmt"
	"strings"
	"unicode"
)

func Filename(title string) (string, error) {
	title = strings.TrimSpace(title)
	if len(title) >= len(".md") && strings.EqualFold(title[len(title)-len(".md"):], ".md") {
		title = title[:len(title)-len(".md")]
	}

	var name strings.Builder
	separator := false
	for _, runeValue := range title {
		if unicode.IsControl(runeValue) || runeValue == '/' || runeValue == '\\' {
			return "", fmt.Errorf("invalid note title %q", title)
		}
		switch {
		case unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue) || unicode.IsMark(runeValue):
			if separator {
				name.WriteByte('-')
				separator = false
			}
			name.WriteRune(unicode.ToLower(runeValue))
		case unicode.IsSpace(runeValue) || unicode.IsPunct(runeValue):
			separator = name.Len() > 0
		}
	}

	result := strings.Trim(name.String(), "-.")
	if result == "" {
		return "", fmt.Errorf("invalid note title %q", title)
	}
	return result + ".md", nil
}
