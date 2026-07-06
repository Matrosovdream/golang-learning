// Package client holds the gateway's gRPC connections to the backend services.
// Each dial installs the request-id client interceptor, so the id the gateway
// mints rides along to catalog and orders.
package client

import (
	"grpcorders/pkg/obs"
	catalogv1 "grpcorders/proto/catalog/v1"
	ordersv1 "grpcorders/proto/orders/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Clients struct {
	Catalog catalogv1.CatalogServiceClient
	Orders  ordersv1.OrderServiceClient
	conns   []*grpc.ClientConn
}

func Dial(catalogAddr, ordersAddr string) (*Clients, error) {
	catalogConn, err := grpc.NewClient(catalogAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(obs.RequestIDUnaryClient),
	)
	if err != nil {
		return nil, err
	}
	ordersConn, err := grpc.NewClient(ordersAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(obs.RequestIDUnaryClient),
	)
	if err != nil {
		_ = catalogConn.Close()
		return nil, err
	}
	return &Clients{
		Catalog: catalogv1.NewCatalogServiceClient(catalogConn),
		Orders:  ordersv1.NewOrderServiceClient(ordersConn),
		conns:   []*grpc.ClientConn{catalogConn, ordersConn},
	}, nil
}

func (c *Clients) Close() {
	for _, conn := range c.conns {
		_ = conn.Close()
	}
}
