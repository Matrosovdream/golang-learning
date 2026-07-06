// Package broker wraps the RabbitMQ client.
package broker

import (
	"context"
	"log"
	"sync"
	"time"

	// Import alias: the package at this path is named amqp091; `amqp` renames it.
	amqp "github.com/rabbitmq/amqp091-go"
)

// The first two fields are pointers (*amqp.Connection, *amqp.Channel): they
// point at values owned by the library, and a nil pointer means "not set yet".
// pubMu is a sync.Mutex held by value — a zero-value Mutex is ready to use.
type Broker struct {
	conn     *amqp.Connection
	pubCh    *amqp.Channel
	pubMu    sync.Mutex
	exchange string
}

// Returns *Broker (a pointer), so callers share one Broker instead of copying
// it. (url, exchange string) groups two params that share a type.
func Connect(url, exchange string, attempts int) (*Broker, error) {
	// `var` with no initializer yields the zero value: nil for a pointer, and
	// nil for an interface type like error.
	var (
		conn *amqp.Connection
		err  error
	)
	for i := 0; i < attempts; i++ {
		conn, err = amqp.Dial(url) // = assigns to the existing vars (not :=)
		if err == nil {
			break
		}
		log.Printf("waiting for broker (attempt %d/%d): %v", i+1, attempts, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel() // := declares ch and reuses err
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		conn.Close()
		return nil, err
	}
	// &Broker{...} takes the address of a new struct literal, giving a *Broker.
	return &Broker{conn: conn, pubCh: ch, exchange: exchange}, nil
}

// Pointer receiver (b *Broker): the method works on the shared Broker and may
// mutate it. A value receiver would operate on a copy.
func (b *Broker) Publish(ctx context.Context, routingKey string, body []byte) error {
	// Lock the mutex; `defer` schedules Unlock to run when the function returns,
	// on every return path.
	b.pubMu.Lock()
	defer b.pubMu.Unlock()
	return b.pubCh.PublishWithContext(ctx, b.exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

// A function type: Handler is any func with this signature, so it can be stored
// in a variable and passed as an argument.
type Handler func(routingKey string, body []byte) error

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
	// range over a slice gives (index, value); _ discards the index here.
	for _, rk := range routingKeys {
		if err := ch.QueueBind(q.Name, rk, b.exchange, false, nil); err != nil {
			return err
		}
	}
	if err := ch.Qos(10, 0, false); err != nil {
		return err
	}
	// deliveries is a channel (<-chan amqp.Delivery): a typed pipe of values.
	deliveries, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	// `go` launches a goroutine — this function literal runs concurrently. It is
	// a closure: it captures `queue` and `handler` from the enclosing scope.
	go func() {
		// range over a channel receives values until the channel is closed.
		for d := range deliveries {
			if err := handler(d.RoutingKey, d.Body); err != nil {
				log.Printf("[%s] handler error for %s: %v", queue, d.RoutingKey, err)
				_ = d.Nack(false, false) // _ = explicitly discards the returned error
			} else {
				_ = d.Ack(false)
			}
		}
	}() // trailing () invokes the function literal immediately

	return nil
}

func (b *Broker) Close() {
	// Guard against a nil pointer before calling a method on it.
	if b.pubCh != nil {
		_ = b.pubCh.Close()
	}
	if b.conn != nil {
		_ = b.conn.Close()
	}
}
