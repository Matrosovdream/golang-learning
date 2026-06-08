// Package client dials the backend gRPC services and exposes typed clients to
// the gateway handlers.
package client

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	catalogv1 "shop/proto/catalog/v1"
	ordersv1 "shop/proto/orders/v1"
	usersv1 "shop/proto/users/v1"
)

// Clients bundles the three downstream gRPC clients and their connections.
type Clients struct {
	Users   usersv1.UsersServiceClient
	Catalog catalogv1.CatalogServiceClient
	Orders  ordersv1.OrdersServiceClient
	conns   []*grpc.ClientConn
}

// Dial creates (lazy) connections to each service. Connections are established
// on the first RPC, so a service being briefly down doesn't fail startup.
func Dial(usersAddr, catalogAddr, ordersAddr string) (*Clients, error) {
	opt := grpc.WithTransportCredentials(insecure.NewCredentials())

	usersConn, err := grpc.NewClient(usersAddr, opt)
	if err != nil {
		return nil, err
	}
	catalogConn, err := grpc.NewClient(catalogAddr, opt)
	if err != nil {
		usersConn.Close()
		return nil, err
	}
	ordersConn, err := grpc.NewClient(ordersAddr, opt)
	if err != nil {
		usersConn.Close()
		catalogConn.Close()
		return nil, err
	}

	return &Clients{
		Users:   usersv1.NewUsersServiceClient(usersConn),
		Catalog: catalogv1.NewCatalogServiceClient(catalogConn),
		Orders:  ordersv1.NewOrdersServiceClient(ordersConn),
		conns:   []*grpc.ClientConn{usersConn, catalogConn, ordersConn},
	}, nil
}

// Close closes every underlying connection.
func (c *Clients) Close() {
	for _, conn := range c.conns {
		_ = conn.Close()
	}
}
