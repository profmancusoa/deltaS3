// Package s3store contains the S3-specific configuration and client helpers
// used by deltaS3 to interact with object storage.
package s3store

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Region         string      `json:"region"`
	Endpoint       string      `json:"endpoint,omitempty"`
	ForcePathStyle bool        `json:"force_path_style,omitempty"`
	Credentials    Credentials `json:"credentials"`
}

type Credentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token,omitempty"`
}

func LoadConfig(path string) (Config, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	// Validate early so the caller gets a local configuration error before any
	// network request is attempted.
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if c.Region == "" {
		return fmt.Errorf("region is required")
	}
	if err := c.Credentials.Validate(); err != nil {
		return err
	}

	return nil
}

func (c Credentials) Validate() error {
	if c.AccessKeyID == "" {
		return fmt.Errorf("credentials.access_key_id is required")
	}
	if c.SecretAccessKey == "" {
		return fmt.Errorf("credentials.secret_access_key is required")
	}

	return nil
}
