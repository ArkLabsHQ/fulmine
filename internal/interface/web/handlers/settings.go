package handlers

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ArkLabsHQ/ark-node/internal/interface/web/types"
)

var defaultSettings = types.Settings{
	ApiRoot:     "https://fulmine.io/api/D9D90N192031",
	AspUrl:      "http://localhost:7000",
	Currency:    "usd",
	EventServer: "http://arklabs.to/node/jupiter29",
	FullNode:    "http://arklabs.to/node/213908123",
	LnConnect:   false,
	LnUrl:       "lndconnect://192.168.1.4:10009",
	Unit:        "sat",
}

var datadir = "./storage"
var filePath = datadir + "/settings.json"

func ReadSettings() (types.Settings, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			WriteSettings(defaultSettings)
		}
		return defaultSettings, err
	}

	if len(data) == 0 {
		return defaultSettings, nil
	}

	var settings types.Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return defaultSettings, err
	}

	return settings, nil
}

func WriteSettings(settings types.Settings) error {
	if err := makeDirectoryIfNotExists(datadir); err != nil {
		return fmt.Errorf("failed to initialize datadir: %s", err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

func SaveAspUrlToSettings(aspurl string) error {
	settings, err := ReadSettings()
	if err != nil {
		return err
	}
	settings.AspUrl = aspurl
	return WriteSettings(settings)
}

func makeDirectoryIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModeDir|0755)
	}
	return nil
}
