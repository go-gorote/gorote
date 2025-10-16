package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	*s3.Client
	Region string
}

func NewS3Storage(cfg Config) (*S3Storage, error) {
	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	return &S3Storage{
		Client: s3.NewFromConfig(awsCfg),
		Region: cfg.Region,
	}, nil
}

func (s *S3Storage) Upload(ctx context.Context, bucket, key string, file io.Reader, contentType string) error {
	uploader := manager.NewUploader(s.Client)
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("failed to upload to s3: %w", err)
	}
	return nil
}

func (s *S3Storage) GetPresignedURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	presigner := s3.NewPresignClient(s.Client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("failed to create presigned url: %w", err)
	}
	return req.URL, nil
}
