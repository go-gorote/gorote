package gorote

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type InitRabbitMQ struct {
	User     string
	Password string
	Host     string
	Port     int
}

func (q *InitRabbitMQ) ConnString() string {
	user := url.QueryEscape(q.User)
	pass := url.QueryEscape(q.Password)
	return fmt.Sprintf("amqp://%s:%s@%s:%d", user, pass, q.Host, q.Port)
}

type ConnRabbitMQ struct {
	URL        string
	Channel    *amqp.Channel
	Connection *amqp.Connection
	mu         sync.Mutex
}

type HandlesRabbitMQ func(context.Context, amqp.Delivery) error

type queueWithString interface {
	ConnString() string
}

func Redelivery(b amqp.Delivery) int {
	count, ok := b.Headers["x-delivery-count"]
	if !ok {
		return 0
	}
	retry, err := strconv.Atoi(fmt.Sprintf("%v", count))
	if err != nil {
		return 0
	}
	return retry
}

func Connect(ctx context.Context, dsn queueWithString, vhost string) (*ConnRabbitMQ, error) {
	vhost = url.PathEscape(vhost)
	urlConn := fmt.Sprintf("%s/%s", dsn.ConnString(), vhost)

	r := &ConnRabbitMQ{
		URL: urlConn,
	}

	if err := r.connect(ctx); err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		r.Close()
	}()

	return r, nil
}

func (r *ConnRabbitMQ) connect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	done := make(chan struct{})
	var err error
	var conn *amqp.Connection

	go func() {
		conn, err = amqp.Dial(r.URL)
		close(done)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("RabbitMQ connection canceled by context: %w", ctx.Err())
	case <-done:
		if err != nil {
			return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
		}
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	r.Connection = conn
	r.Channel = ch

	go func() {
		errChan := make(chan *amqp.Error, 1)
		r.Connection.NotifyClose(errChan)
		if err := <-errChan; err != nil {
			log.Printf("[RabbitMQ] connection closed: %v", err)
		}
	}()

	return nil
}

func (r *ConnRabbitMQ) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Channel != nil {
		_ = r.Channel.Close()
	}
	if r.Connection != nil && !r.Connection.IsClosed() {
		_ = r.Connection.Close()
	}
}

func (r *ConnRabbitMQ) Reconnect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var err error

	if r.Connection != nil && !r.Connection.IsClosed() {
		_ = r.Connection.Close()
	}

	for i := 1; i <= 5; i++ {
		log.Printf("[RabbitMQ] Attempting to reconnect... (%d/5)", i)
		err = r.connect(ctx)
		if err == nil {
			log.Println("[RabbitMQ] Successfully reconnected!")
			return nil
		}
		log.Printf("[RabbitMQ] Failed to reconnect: %v", err)
		time.Sleep(time.Duration(i) * 3 * time.Second)
	}

	return fmt.Errorf("failed to reconnect after multiple attempts: %w", err)
}

func (r *ConnRabbitMQ) ConsumerMessages(ctx context.Context, worker int, queue, nameConsumer string, handler HandlesRabbitMQ, errHandlers ...HandlesRabbitMQ) error {
	for {
		if err := r.Channel.Qos(worker, 0, false); err != nil {
			return fmt.Errorf("failed to configure QoS: %w", err)
		}

		msgs, err := r.Channel.ConsumeWithContext(ctx, queue, nameConsumer, false, false, false, false, nil)
		if err != nil {
			return fmt.Errorf("failed to register consumer: %w", err)
		}

		sem := make(chan struct{}, worker)
		var wg sync.WaitGroup

		for {
			select {
			case <-ctx.Done():
				wg.Wait()
				return fmt.Errorf("[RabbitMQ] context closed, shutting down consumer")

			case d, ok := <-msgs:
				if !ok {
					log.Println("[RabbitMQ] channel closed, attempting to reconnect...")
					time.Sleep(3 * time.Second)
					if err := r.Reconnect(ctx); err != nil {
						time.Sleep(5 * time.Second)
						continue
					}
					break
				}

				sem <- struct{}{}
				wg.Add(1)
				go func(msg amqp.Delivery) {
					defer func() {
						<-sem
						wg.Done()
					}()

					if err := handler(ctx, msg); err != nil {
						_ = msg.Nack(false, true)
						for _, errHandler := range errHandlers {
							if e := errHandler(ctx, msg); e != nil {
								return
							}
						}
						return
					}
					_ = msg.Ack(false)
				}(d)
			}
		}
	}
}

func (r *ConnRabbitMQ) PublishStruct(ctx context.Context, queueName string, data any) error {
	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize struct: %w", err)
	}

	err = r.Channel.PublishWithContext(ctx,
		"",
		queueName,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
	if err != nil {
		log.Printf("[RabbitMQ] failed to publish: %v — attempting to reconnect", err)
		if recErr := r.Reconnect(ctx); recErr == nil {
			err = r.Channel.PublishWithContext(ctx,
				"",
				queueName,
				false,
				false,
				amqp.Publishing{
					ContentType: "application/json",
					Body:        body,
				})
		}
	}

	if err != nil {
		return fmt.Errorf("failed to publish to queue: %w", err)
	}
	return nil
}
