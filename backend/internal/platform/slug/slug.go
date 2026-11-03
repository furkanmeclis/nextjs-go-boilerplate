package slug

import (
	"regexp"
	"strings"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// turkishFold maps Turkish-specific letters (both cases) to their ASCII
// equivalents. Without it the ASCII-only slug regex strips these letters, so
// "Çağrı Şirketi" collapsed to "a-r-irketi". Folding the uppercase forms here
// (before strings.ToLower) also makes the result independent of how ToLower
// happens to case-map letters like "İ" (U+0130) — we pin them to a plain "i".
var turkishFold = strings.NewReplacer(
	"İ", "i", "I", "i", "ı", "i",
	"Ş", "s", "ş", "s",
	"Ğ", "g", "ğ", "g",
	"Ç", "c", "ç", "c",
	"Ö", "o", "ö", "o",
	"Ü", "u", "ü", "u",
)

// FromName turns a display name into a URL-safe slug (ASCII-oriented).
func FromName(name string) string {
	name = turkishFold.Replace(strings.TrimSpace(name))
	name = strings.ToLower(name)
	if name == "" {
		return "item"
	}
	slug := nonSlug.ReplaceAllString(name, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "item"
	}
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	return slug
}
