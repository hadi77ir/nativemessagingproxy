package config

import (
	"encoding/json"
	"io"
	"os"

	"sigs.k8s.io/yaml"
)

type Config struct {
	Command string `json:"command"`
	Proxy   string `json:"proxy"`
	LogPath string `json:"log"`
}

func ConfigPath() string {
	path := os.Getenv("NMPROXY_CONFIG")
	if path == "" {
		userPath, err := os.UserConfigDir()
		if err != nil {
			return "/etc/nmproxy.cfg"
		}
		path = userPath + "/nmproxy.cfg"
	}
	return path
}

func ReadConfig() *Config {
	configPath := ConfigPath()
	f, err := os.Open(configPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	var config Config
	bytes, err := io.ReadAll(f)
	if err != nil {
		return nil
	}
	if err := yaml.Unmarshal(bytes, &config); err != nil {
		// try json
		if err := json.Unmarshal(bytes, &config); err != nil {
			return nil
		}
	}
	return &config
}
func FailsafeReadConfig() *Config {
	cfg := ReadConfig()
	if cfg == nil {
		return &emptyConfig
	}
	return cfg
}

func EmptyConfig() *Config {
	return &emptyConfig
}

var emptyConfig Config = Config{
	Proxy:   "",
	Command: "",
	LogPath: "stderr",
}
