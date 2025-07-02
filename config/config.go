package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

type Config struct {
	S3Endpoint        string `yaml:"s3_endpoint"`
	S3Bucket          string `yaml:"s3_bucket"`
	S3AccessKey       string `yaml:"s3_access_key"`
	S3SecretKey       string `yaml:"s3_secret_key"`
	PrivateGPGKey     string `yaml:"private_gpg_key"`
	PrivateGPGPasskey string `yaml:"private_gpg_passkey"`
}

func New() *Config {
	return &Config{}
}

func (c *Config) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}

	return nil
}

func (c *Config) Validate() error {
	return nil
}
