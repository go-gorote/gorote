package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStorage struct {
	client *minio.Client
}

func NewMinIOStorage(cfg Config) (*MinIOStorage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}
	return &MinIOStorage{client: client}, nil
}

func (m *MinIOStorage) Upload(ctx context.Context, bucket, key string, file io.Reader, contentType string) error {
	_, err := m.client.PutObject(ctx, bucket, key, file, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to upload object: %w", err)
	}

	return nil
}

func (m *MinIOStorage) GetPresignedURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	reqParams := make(map[string][]string)
	url, err := m.client.PresignedGetObject(ctx, bucket, key, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return url.String(), nil
}
