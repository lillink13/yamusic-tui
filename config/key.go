package config

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"gopkg.in/yaml.v3"
)

type Key struct {
	displayName string
	keyNames    []string
}

func NewKey(key string) *Key {
	k := &Key{
		displayName: prepareToDisplay(key),
		keyNames:    prepareToProccess(key),
	}
	return k
}

func (k *Key) IsEmpty() bool {
	return k == nil || len(k.keyNames) == 0
}

func (k *Key) Binding() key.BindingOpt {
	return key.WithKeys(k.keyNames...)
}

func (k *Key) Help(help string) key.BindingOpt {
	return key.WithHelp(k.displayName, help)
}

func (k *Key) Contains(keyName string) bool {
	// Match layout-independent: a Cyrillic keystroke (RU layout) is compared
	// against its physical Latin key, so a hotkey configured as "c" fires when
	// the user presses the physical C key with either layout active.
	return slices.Contains(k.keyNames, normalizeLayout(keyName))
}

// normalizeLayout maps Cyrillic letters to the Latin letters on the same
// physical keys (ЙЦУКЕН layout), preserving case, so bindings are independent
// of the active keyboard layout. Non-letter runes and Latin text pass through
// unchanged; Cyrillic symbols mapped to non-letter keys (e.g. х->[) are kept
// as-is for letter-only hotkeys anyway.
func normalizeLayout(text string) string {
	var b strings.Builder
	for _, r := range text {
		lat, ok := cyrillicToLatin[r]
		if !ok {
			b.WriteRune(r)
			continue
		}
		// Preserve case only when both sides are letters.
		b.WriteRune(lat)
	}
	return b.String()
}

// cyrillicToLatin maps lowercase Cyrillic to the Latin letter occupying the
// same physical key in the standard Russian (ЙЦУКЕН) layout.
var cyrillicToLatin = map[rune]rune{
	'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y',
	'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']',
	'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h',
	'о': 'j', 'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'',
	'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n',
	'ь': 'm', 'б': ',', 'ю': '.',
	'Й': 'q', 'Ц': 'w', 'У': 'e', 'К': 'r', 'Е': 't', 'Н': 'y',
	'Г': 'u', 'Ш': 'i', 'Щ': 'o', 'З': 'p', 'Х': '[', 'Ъ': ']',
	'Ф': 'a', 'Ы': 's', 'В': 'd', 'А': 'f', 'П': 'g', 'Р': 'h',
	'О': 'j', 'Л': 'k', 'Д': 'l', 'Ж': ';', 'Э': '\'',
	'Я': 'z', 'Ч': 'x', 'С': 'c', 'М': 'v', 'И': 'b', 'Т': 'n',
	'Ь': 'm', 'Б': ',', 'Ю': '.',
}

func (k *Key) MarshalYAML() (interface{}, error) {
	return strings.Join(k.keyNames, ","), nil
}

func (k *Key) UnmarshalYAML(val *yaml.Node) error {
	k.displayName = prepareToDisplay(val.Value)
	k.keyNames = prepareToProccess(val.Value)
	return nil
}

func prepareToProccess(key string) []string {
	var s = strings.ReplaceAll(key, "space", " ")
	s = strings.ReplaceAll(s, "↑", "up")
	s = strings.ReplaceAll(s, "↓", "down")
	s = strings.ReplaceAll(s, "←", "left")
	s = strings.ReplaceAll(s, "→", "right")
	tokens := strings.Split(s, ",")
	for i := range tokens {
		tokens[i] = normalizeShiftLetter(tokens[i])
	}
	return tokens
}

// normalizeShiftLetter rewrites a "shift+<letter>" binding to its uppercase
// letter (e.g. "shift+f" -> "F"). Terminals deliver a shifted letter as the
// uppercase rune, not as a "shift+" modifier event, so a literal "shift+f"
// binding would never match. Other shifted keys ("shift+up", …) and multi-rune
// tokens are left untouched.
func normalizeShiftLetter(token string) string {
	rest, ok := strings.CutPrefix(token, "shift+")
	if !ok || len(rest) != 1 {
		return token
	}
	if c := rest[0]; c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
		return strings.ToUpper(rest)
	}
	return token
}

func prepareToDisplay(key string) string {
	var s = strings.ReplaceAll(key, " ", "space")
	s = strings.ReplaceAll(s, "up", "↑")
	s = strings.ReplaceAll(s, "down", "↓")
	s = strings.ReplaceAll(s, "left", "←")
	s = strings.ReplaceAll(s, "right", "→")
	return s
}
