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
	Channel    *amqp.Channel
	Connection *amqp.Connection
}

func (r *InitRabbitMQ) ConnectRabbitMQ(conn *ConnRabbitMQ, vhost string, connectionName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	vhost = url.PathEscape(vhost)
	urlConn := fmt.Sprintf("%s/%s", r.ConnString(), vhost)

	resultChan := make(chan error, 1)

	go func() {
		newConn, err := amqp.DialConfig(urlConn, amqp.Config{
			Properties: amqp.Table{
				"connection_name": connectionName,
			},
		})
		if err != nil {
			resultChan <- err
			return
		}

		ch, err := newConn.Channel()
		if err != nil {
			newConn.Close()
			resultChan <- err
			return
		}

		conn.Connection = newConn
		conn.Channel = ch

		go func() {
			closeErr := make(chan *amqp.Error, 1)
			conn.Connection.NotifyClose(closeErr)

			if err := <-closeErr; err != nil {
				log.Printf("[RabbitMQ] conexão fechada: %v — tentando reconectar...", err)

				for {
					time.Sleep(5 * time.Second)

					if err := r.ConnectRabbitMQ(conn, vhost, connectionName); err == nil {
						log.Println("[RabbitMQ] reconectado com sucesso!")
						break
					}

					log.Printf("[RabbitMQ] falha ao reconectar: %v — nova tentativa em 5s", err)
				}
			}
		}()

		resultChan <- nil
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("timeout ao conectar ao RabbitMQ após 10s")
	case err := <-resultChan:
		return err
	}
}

func (r *ConnRabbitMQ) Consumer(ctx context.Context, worker int, queue, nameConsumer string, f func(delivery amqp.Delivery) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := r.Channel.Qos(worker, 0, false); err != nil {
			log.Printf("[RabbitMQ] Erro ao configurar QoS: %v", err)
			continue
		}

		msgs, err := r.Channel.ConsumeWithContext(ctx, queue, nameConsumer, false, false, false, false, nil)
		if err != nil {
			log.Printf("[RabbitMQ] Erro ao registrar consumer: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[RabbitMQ] Consumer registrado com sucesso na fila %s", queue)

		if err := r.processMessages(ctx, worker, msgs, f); err != nil {
			log.Printf("[RabbitMQ] processMessages retornou erro: %v - reconectando...", err)
		}

		time.Sleep(2 * time.Second)
	}
}

func (r *ConnRabbitMQ) processMessages(ctx context.Context, worker int, msgs <-chan amqp.Delivery, f func(delivery amqp.Delivery) error) error {
	sem := make(chan struct{}, worker)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			log.Println("[RabbitMQ] Contexto cancelado, aguardando finalização dos workers...")
			wg.Wait()
			return nil

		case d, ok := <-msgs:
			if !ok {
				log.Println("[RabbitMQ] Canal de mensagens fechado")
				return fmt.Errorf("[RabbitMQ] canal de mensagens fechado")
			}

			sem <- struct{}{}
			wg.Add(1)
			go func(msg amqp.Delivery) {
				defer func() {
					<-sem
					wg.Done()
				}()

				if err := f(msg); err != nil {
					log.Printf("[RabbitMQ] Erro no handler: %v", err)
					return
				}
				if err := msg.Ack(false); err != nil {
					log.Printf("[RabbitMQ] Erro ao fazer ACK: %v", err)
				}
			}(d)
		}
	}
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

func (r *ConnRabbitMQ) Publish(ctx context.Context, queueName string, data any) error {
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
			Timestamp:   time.Now(),
		})

	if err != nil {
		return fmt.Errorf("falha ao publicar na fila: %w", err)
	}

	log.Printf("[RabbitMQ] Mensagem publicada com sucesso na fila %s", queueName)
	return nil
}
