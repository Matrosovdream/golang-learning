package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"eventshop/pkg/broker"
	"eventshop/pkg/db"
	"eventshop/pkg/events"
	"eventshop/services/notifications/internal/service"
)

const exchange = "shop.events"

func main() {
	amqpURL := db.Getenv("AMQP_URL", "amqp://guest:guest@localhost:5672/")

	bus, err := broker.Connect(amqpURL, exchange, 30)
	if err != nil {
		log.Fatalf("broker: %v", err)
	}
	defer bus.Close()

	svc := service.New()

	err = bus.Consume("notifications.events",
		[]string{events.PaymentSettled, events.StockRejected},
		func(routingKey string, body []byte) error {
			return svc.Handle(routingKey, body)
		})
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	log.Println("notifications consuming payment.settled + stock.rejected")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("notifications shutting down...")
}
