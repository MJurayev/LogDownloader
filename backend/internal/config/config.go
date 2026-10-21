package config

import (
	"flag"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Host      string `yaml:"host"`
	Port      string `yaml:"port"`
	DataDir   string `yaml:"data_dir"`
	JWTSecret string `yaml:"jwt_secret"`
	// ExportChunkMB — avtomatik bo'laklash: .tar.gz ichidagi bitta .log qismning
	// maksimal xom hajmi (MB). Export shundan katta bo'lsa qator chegarasida
	// `<nom>_partNNN.log` qismlarga bo'linadi. 0 = bo'laklash o'chiq (bitta fayl).
	ExportChunkMB int `yaml:"export_chunk_mb"`
}

func Load() Config {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg := Config{
		Host:          "",
		Port:          "3000",
		DataDir:       "/var/lib/logdownloader",
		ExportChunkMB: 100,
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
	if v := os.Getenv("EXPORT_CHUNK_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.ExportChunkMB = n
		}
	}
	if cfg.ExportChunkMB < 0 {
		cfg.ExportChunkMB = 0
	}

	return cfg
}
