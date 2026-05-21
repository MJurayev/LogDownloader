package config

import (
	"flag"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Host      string `yaml:"host"`
	Port      string `yaml:"port"`
	DataDir   string `yaml:"data_dir"`
	JWTSecret string `yaml:"jwt_secret"`
}

func Load() Config {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg := Config{
		Host:    "",
		Port:    "3000",
		DataDir: "/var/lib/logdownloader",
	}

	// 1. Config file
	path := *configPath
	if path == "" {
		path = os.Getenv("CONFIG_PATH")
	}
	if path == "" {
		// default locations
		for _, p := range []string{"config.yaml", "/etc/logdownloader/config.yaml"} {
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			yaml.Unmarshal(data, &cfg)
		}
	}

	// 2. Env vars override
	if v := os.Getenv("HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}

	return cfg
}
