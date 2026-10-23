// Package i18n carries the app's interface languages, embedded as
// languages.json.
package i18n

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed languages.json
var languagesJSON []byte

// Dict is one language's messages.
type Dict map[string]string

var (
	loaded   map[string]Dict
	loadOnce sync.Once
)

func load() {
	loadOnce.Do(func() {
		raw := map[string]map[string]any{}
		if err := json.Unmarshal(languagesJSON, &raw); err != nil {
			loaded = map[string]Dict{"en": {}}
			return
		}
		loaded = make(map[string]Dict, len(raw))
		for locale, messages := range raw {
			dict := make(Dict, len(messages))
			for key, value := range messages {
				if s, ok := value.(string); ok {
					dict[key] = s
				}
			}
			loaded[locale] = dict
		}
	})
}

// Locales lists the languages the app is translated into.
func Locales() []string {
	load()
	return []string{"en", "zh", "zh_hant", "fr", "de", "ja", "tr", "ko", "pl", "es", "it", "nl", "pt", "vi", "ru", "th"}
}

// LocaleNames maps a locale key to its own name, for the language menu.
func LocaleNames() map[string]string {
	return map[string]string{
		"en":      "English",
		"zh":      "简体中文",
		"zh_hant": "繁體中文",
		"fr":      "Français",
		"de":      "Deutsch",
		"ja":      "日本語",
		"tr":      "Türkçe",
		"ko":      "한국어",
		"pl":      "Polski",
		"es":      "Español",
		"it":      "Italiano",
		"nl":      "Nederlands",
		"pt":      "Português",
		"vi":      "Tiếng Việt",
		"ru":      "Русский",
		"th":      "ไทย",
	}
}

// Normalize maps a locale tag the app was told to one of its keys.
func Normalize(locale string) string {
	locale = strings.ToLower(strings.ReplaceAll(locale, "_", "-"))
	switch {
	case locale == "":
		return ""
	case strings.HasPrefix(locale, "zh-hant"), strings.HasPrefix(locale, "zh-tw"),
		strings.HasPrefix(locale, "zh-hk"), strings.HasPrefix(locale, "zh-mo"):
		return "zh_hant"
	case strings.HasPrefix(locale, "zh"):
		return "zh"
	case strings.HasPrefix(locale, "en"):
		return "en"
	case strings.HasPrefix(locale, "fr"):
		return "fr"
	case strings.HasPrefix(locale, "de"):
		return "de"
	case strings.HasPrefix(locale, "ja"):
		return "ja"
	case strings.HasPrefix(locale, "tr"):
		return "tr"
	case strings.HasPrefix(locale, "ko"):
		return "ko"
	case strings.HasPrefix(locale, "pl"):
		return "pl"
	case strings.HasPrefix(locale, "es"):
		return "es"
	case strings.HasPrefix(locale, "it"):
		return "it"
	case strings.HasPrefix(locale, "nl"):
		return "nl"
	case strings.HasPrefix(locale, "pt"):
		return "pt"
	case strings.HasPrefix(locale, "vi"):
		return "vi"
	case strings.HasPrefix(locale, "ru"):
		return "ru"
	case strings.HasPrefix(locale, "th"):
		return "th"
	}
	return ""
}

// Detect picks the language from the system's locale tag.
func Detect(systemLocale string) string {
	if key := Normalize(systemLocale); key != "" {
		return key
	}
	return "en"
}

// Translator answers the messages of one language, falling back to English.
type Translator struct {
	Locale string
	dict   Dict
	base   Dict
}

// New returns a translator for locale.
func New(locale string) *Translator {
	load()
	key := Normalize(locale)
	if key == "" {
		key = "en"
	}
	dict, ok := loaded[key]
	if !ok {
		dict, key = loaded["en"], "en"
	}
	return &Translator{Locale: key, dict: dict, base: loaded["en"]}
}

// T returns the message for key, or its English text, or the key itself.
func (t *Translator) T(key string) string {
	if v, ok := t.dict[key]; ok {
		return v
	}
	if v := native(t.Locale, key); v != "" {
		return v
	}
	if v, ok := t.base[key]; ok {
		return v
	}
	return key
}

// Args is the argument list of a message with placeholders.
type Args []string

// F returns the message for key with its {0}, {1} placeholders filled.
func (t *Translator) F(key string, args ...string) string {
	s := t.T(key)
	for i, arg := range args {
		if strings.Contains(s, "%s") && i == 0 {
			s = strings.Replace(s, "%s", arg, 1)
			continue
		}
		s = strings.ReplaceAll(s, "{"+itoa(i)+"}", arg)
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
