package server

import (
	"crypto/tls"
	"time"

	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/lang"
)

const (
	defaultAddress  = "0.0.0.0:8080"
	defaultEndpoint = "/webhook"
	defaultTimeout  = 30 * time.Second
)

// Config represents webhook server configuration
type Config struct {
	Address  string        `yaml:"address" env:"SERVER_ADDRESS"`
	Endpoint string        `yaml:"endpoint" env:"SERVER_ENDPOINT"`
	Timeout  time.Duration `yaml:"timeout" env:"SERVER_TIMEOUT"`

	CertFilePath string `yaml:"cert_file_path" env:"CERT_FILE_PATH"`
	KeyFilePath  string `yaml:"key_file_path" env:"KEY_FILE_PATH"`
	EnableHTTPS  bool   `yaml:"enable_https" env:"SERVER_ENABLE_HTTPS"`

	Certificate tls.Certificate `yaml:"-"`
}

func (cfg *Config) PrepareAndValidate() error {
	cfg.Address = lang.Check(cfg.Address, defaultAddress)
	cfg.Endpoint = lang.Check(cfg.Endpoint, defaultEndpoint)

	if cfg.EnableHTTPS {
		if cfg.CertFilePath == "" || cfg.KeyFilePath == "" {
			return errm.New("cert_file_path and key_file_path must be set when enable_https is true")
		}

		cert, err := tls.LoadX509KeyPair(cfg.CertFilePath, cfg.KeyFilePath)
		if err != nil {
			return errm.Wrap(err, "failed to load certificate and key pair")
		}

		cfg.Certificate = cert
	}

	return nil
}
