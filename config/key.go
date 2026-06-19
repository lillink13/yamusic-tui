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
	return slices.Contains(k.keyNames, keyName)
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
