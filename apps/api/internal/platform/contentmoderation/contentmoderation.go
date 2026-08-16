package contentmoderation

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

var (
	tagPattern   = regexp.MustCompile(`<[^>]*>`)
	urlPattern   = regexp.MustCompile(`(?i)https?://|www\.`)
	spacePattern = regexp.MustCompile(`\s+`)
)

// LooksLikeJunk is deliberately conservative. It rejects obvious placeholders,
// repeated-character garbage and URL dumps before a marketplace entity reaches
// persistent storage. Borderline content is left for normal moderation.
func LooksLikeJunk(parts ...string) bool {
	raw := strings.TrimSpace(strings.Join(parts, " "))
	if raw == "" {
		return true
	}
	plain := strings.ToLower(html.UnescapeString(tagPattern.ReplaceAllString(raw, " ")))
	plain = strings.TrimSpace(spacePattern.ReplaceAllString(plain, " "))
	compact := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, plain)
	if len([]rune(compact)) >= 5 {
		for _, placeholder := range []string{"qwerty", "asdfgh", "йцукен", "тесттесттест", "testtesttest", "111111", "123123123"} {
			if compact == placeholder || strings.HasPrefix(compact, placeholder+placeholder) {
				return true
			}
		}
	}
	if len(urlPattern.FindAllStringIndex(raw, -1)) > 5 {
		return true
	}
	runes := []rune(compact)
	if len(runes) >= 12 {
		counts := map[rune]int{}
		max := 0
		for _, r := range runes {
			counts[r]++
			if counts[r] > max {
				max = counts[r]
			}
		}
		if float64(max)/float64(len(runes)) >= .78 {
			return true
		}
	}
	letters := 0
	for _, r := range runes {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return len(runes) >= 8 && letters == 0
}
