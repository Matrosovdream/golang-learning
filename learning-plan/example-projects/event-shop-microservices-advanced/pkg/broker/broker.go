// Package broker is a thin RabbitMQ helper: connect with retry, publish to a
// topic exchange, and consume a durable queue bound to routing keys.
package broker

import (
	"context"
	"log"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Broker wraps a connection plus a dedicated publishing channel. Each consumer
// gets its own channel (amqp channels are not safe for concurrent use).
type Broker struct {
	conn     *amqp.Connection
	pubCh    *amqp.Channel
	pubMu    sync.Mutex
	exchange string
}

// Connect dials RabbitMQ (retrying), opens a publish channel, and declares the
// durable topic exchange.
func Connect(url, exchange string, attempts int) (*Broker, error) {
	var (
		conn *amqp.Connection
		err  error
	)
	for i := 0; i < attempts; i++ {
		conn, err = amqp.Dial(url)
		if err == nil {
			break
		}
		log.Printf("waiting for broker (attempt %d/%d): %v", i+1, attempts, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		conn.Close()
		return nil, err
	}
	return &Broker{conn: conn, pubCh: ch, exchange: exchange}, nil
}

// Publish sends a JSON body to the exchange under routingKey (persistent).
func (b *Broker) Publish(ctx context.Context, routingKey string, body []byte) error {
	b.pubMu.Lock()
	defer b.pubMu.Unlock()
	return b.pubCh.PublishWithContext(ctx, b.exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

// Handler processes one delivery. Returning nil acks it; an error nacks it
// without requeue (so a poison message doesn't loop forever).
type Handler func(routingKey string, body []byte) error

// Consume declares a durable queue bound to routingKeys on its own channel and
// dispatches deliveries to handler in a background goroutine.
func (b *Broker) Consume(queue string, routingKeys []string, handler Handler) error {
	ch, err := b.conn.Channel()
	if err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(b.exchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	q, err := ch.QueueDeclare(queue, true, false, false, false, nil)
	if err != nil {
		return err
	}
	for _, rk := range routingKeys {
		if err := ch.QueueBind(q.Name, rk, b.exchange, false, nil); err != nil {
			return err
		}
	}
	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	go func() {
		for d := range deliveries {
			if err := handler(d.RoutingKey, d.Body); err != nil {
				log.Printf("[%s] handler error for %s: %v", queue, d.RoutingKey, err)
				_ = d.Nack(false, false)
			} else {
				_ = d.Ack(false)
			}
		}
	}()
	return nil
}

// Close shuts down the publish channel and connection.
func (b *Broker) Close() {
	if b.pubCh != nil {
		_ = b.pubCh.Close()
	}
	if b.conn != nil {
		_ = b.conn.Close()
	}
}
