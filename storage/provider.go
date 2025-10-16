package storage

import (
	"context"
	"io"
	"time"
)

type StorageProvider interface {
	Upload(context.Context, string, string, io.Reader, string) error
	GetPresignedURL(context.Context, string, string, time.Duration) (string, error)
}
