package gorote

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type InitSQS struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

type ConnSQS struct {
	*sqs.Client
}

type HandlesSQS func(context.Context, types.Message) error

func (s *InitSQS) Connect(ctx context.Context) (*ConnSQS, error) {
	if s.Region == "" || s.AccessKeyID == "" || s.SecretAccessKey == "" {
		return nil, fmt.Errorf("credenciais inválidas")
	}
	customConfig, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(s.Region),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     s.AccessKeyID,
				SecretAccessKey: s.SecretAccessKey,
				SessionToken:    s.SessionToken,
			},
		}),
	)
	if err != nil {
		return nil, err
	}
	return &ConnSQS{sqs.NewFromConfig(customConfig)}, nil
}

func (s ConnSQS) ConsumerMessages(ctx context.Context, worker int, queueURL string, handler HandlesSQS, errHandlers ...HandlesSQS) error {
	if worker > 10 || worker <= 0 {
		return fmt.Errorf("quantidade de workers inválida min: 1, max: 10")
	}
	sem := make(chan struct{}, worker)
	var wg sync.WaitGroup
	for {
		resp, err := s.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &queueURL,
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     5,
		})
		if err != nil {
			time.Sleep(2 * time.Second)
			return err
		}

		for _, msg := range resp.Messages {
			select {
			case <-ctx.Done():
				wg.Wait()
				return fmt.Errorf("contexto encerrado. Finalizando leitura da fila")
			case sem <- struct{}{}:
				wg.Add(1)
				go func(m types.Message) {
					defer func() {
						<-sem
						wg.Done()
					}()
					if err := handler(ctx, m); err != nil {
						for _, errHandler := range errHandlers {
							if err := errHandler(ctx, msg); err != nil {
								return
							}
						}
						return
					}
					_, err := s.DeleteMessage(ctx, &sqs.DeleteMessageInput{
						QueueUrl:      &queueURL,
						ReceiptHandle: m.ReceiptHandle,
					})
					if err != nil {
						for _, errHandler := range errHandlers {
							if err := errHandler(ctx, msg); err != nil {
								return
							}
						}
						return
					}
				}(msg)
			}
		}
	}
}
