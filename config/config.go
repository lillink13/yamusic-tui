package config

import (
	"os"
	"path/filepath"
	"reflect"

	"gopkg.in/yaml.v3"
)

var Current Config

func InitialLoad() error {
	var err error

	Current, err = load()
	if err != nil {
		configDir, err := getDir()
		if err != nil {
			return err
		}

		if oldToken, err := os.ReadFile(filepath.Join(configDir, "token")); err == nil {
			Current.Token = string(oldToken)
		}

		save(Current)
	}

	return nil
}

func getDir() (string, error) {
	userDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(userDir, ".config", DirName)
	err = os.MkdirAll(configDir, 0755)
	if err != nil {
		return "", err
	}

	return configDir, nil
}

func load() (Config, error) {
	configDir, err := getDir()
	if err != nil {
		return defaultConfig, err
	}

	configContent, err := os.ReadFile(filepath.Join(configDir, "config.yaml"))
	if err != nil {
		return defaultConfig, err
	}

	return parseConfig(configContent)
}

// parseConfig unmarshals YAML config content and backfills defaults for every
// field the user omitted. It is split out from load() so the merge logic can be
// unit-tested without touching the filesystem.
func parseConfig(content []byte) (Config, error) {
	var newConfig Config
	err := yaml.Unmarshal(content, &newConfig)
	if err != nil {
		return defaultConfig, err
	}

	// Top-level scalar defaults. A partial config (e.g. one written by an older
	// version that lacked these keys) must not silently end up muted, with a
	// zero playback buffer, or with no rewind step.
	if newConfig.Volume == 0 {
		newConfig.Volume = defaultConfig.Volume
	}
	if newConfig.VolumeStep == 0 {
		newConfig.VolumeStep = defaultConfig.VolumeStep
	}
	if newConfig.BufferSize == 0 {
		newConfig.BufferSize = defaultConfig.BufferSize
	}
	if newConfig.RewindDuration == 0 {
		newConfig.RewindDuration = defaultConfig.RewindDuration
	}

	if newConfig.Search == nil {
		search := *defaultConfig.Search
		newConfig.Search = &search
	}

	if newConfig.Controls == nil {
		controls := *defaultConfig.Controls
		newConfig.Controls = &controls
	} else {
		// Backward compatibility: older versions persisted the "show all keys"
		// control under a misspelled key ("show-all-kyes"). Honor it when the
		// corrected key is absent, before defaults are filled in.
		if newConfig.Controls.ShowAllKeys == nil {
			var legacy struct {
				Controls struct {
					ShowAllKeys *Key `yaml:"show-all-kyes"`
				} `yaml:"controls"`
			}
			if yaml.Unmarshal(content, &legacy) == nil && legacy.Controls.ShowAllKeys != nil {
				newConfig.Controls.ShowAllKeys = legacy.Controls.ShowAllKeys
			}
		}
		fillDefault(newConfig.Controls, defaultConfig.Controls)
		if newConfig.Controls.Quit.IsEmpty() {
			newConfig.Controls.Quit = defaultConfig.Controls.Quit
		}
	}

	if newConfig.Style == nil {
		style := *defaultConfig.Style
		newConfig.Style = &style
	} else {
		if newConfig.Style.Icons == nil {
			icons := *defaultConfig.Style.Icons
			newConfig.Style.Icons = &icons
		} else {
			fillDefault(newConfig.Style.Icons, defaultConfig.Style.Icons)
		}
		if newConfig.Style.Colors == nil {
			colors := *defaultConfig.Style.Colors
			newConfig.Style.Colors = &colors
		} else {
			fillDefault(newConfig.Style.Colors, defaultConfig.Style.Colors)
		}
		if newConfig.Style.SidePanelWidth == 0 {
			newConfig.Style.SidePanelWidth = defaultConfig.Style.SidePanelWidth
		}
	}

	return newConfig, nil
}

func fillDefault(target, values any) {
	targetStruct := reflect.ValueOf(target).Elem()
	defaultStruct := reflect.ValueOf(values).Elem()
	for i := 0; i < targetStruct.NumField(); i++ {
		field := targetStruct.Field(i)
		if field.IsZero() || field.String() == "" {
			field.Set(defaultStruct.Field(i))
		}
	}
}

func save(conf Config) error {
	configDir, err := getDir()
	if err != nil {
		return err
	}

	file, err := os.OpenFile(filepath.Join(configDir, "config.yaml"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := yaml.NewEncoder(file)
	enc.SetIndent(4)
	err = enc.Encode(conf)
	if err != nil {
		return err
	}

	return nil
}

func Path() string {
	configDir, err := getDir()
	if err != nil {
		return ""
	}

	return filepath.Join(configDir, "config.yaml")
}

func Save() error {
	return save(Current)
}

func Reset() error {
	var err error
	Current, err = load()
	return err
}
