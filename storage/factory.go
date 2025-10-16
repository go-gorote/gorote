package storage

import (
	"fmt"
)

type ProviderType string

const (
	ProviderMinIO ProviderType = "minio"
	ProviderS3    ProviderType = "s3"
)

type Config struct {
	Provider  ProviderType
	Endpoint  string
	AccessKey string
	SecretKey string
	Region    string
	UseSSL    bool
}

func NewStorage(cfg Config) (StorageProvider, error) {
	switch cfg.Provider {
	case ProviderMinIO:
		return NewMinIOStorage(cfg)
	case ProviderS3:
		return NewS3Storage(cfg)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}
