// Package server is the orders gRPC transport. CreateOrder fans out to catalog.
package server

import (
	"context"
	"errors"

	catalogv1 "grpcobs/proto/catalog/v1"
	ordersv1 "grpcobs/proto/orders/v1"
	"grpcobs/services/orders/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	ordersv1.UnimplementedOrderServiceServer
	repo    domain.OrderRepository
	catalog catalogv1.CatalogServiceClient
}

func New(repo domain.OrderRepository, catalog catalogv1.CatalogServiceClient) *Server {
	return &Server{repo: repo, catalog: catalog}
}

func (s *Server) CreateOrder(ctx context.Context, r *ordersv1.CreateOrderRequest) (*ordersv1.Order, error) {
	if len(r.GetItems()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "order must have at least one item")
	}

	order := domain.Order{Status: "confirmed"}
	for _, line := range r.GetItems() {
		if line.GetQuantity() <= 0 {
			return nil, status.Error(codes.InvalidArgument, "quantity must be positive")
		}
		// Fan-out over gRPC. ctx carries the request-id (forwarded by the client
		// interceptor), so catalog logs it too.
		res, err := s.catalog.ReserveStock(ctx, &catalogv1.ReserveStockRequest{
			ProductId: line.GetProductId(),
			Quantity:  line.GetQuantity(),
		})
		if err != nil {
			return nil, err // propagate catalog's status code across the boundary
		}
		order.Items = append(order.Items, domain.OrderItem{
			ProductID:  line.GetProductId(),
			Quantity:   line.GetQuantity(),
			Name:       res.GetName(),
			PriceCents: res.GetPriceCents(),
		})
		order.TotalCents += res.GetPriceCents() * int64(line.GetQuantity())
	}

	saved, err := s.repo.Create(ctx, order)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not save order")
	}
	return toProto(saved), nil
}

func (s *Server) GetOrder(ctx context.Context, r *ordersv1.GetOrderRequest) (*ordersv1.Order, error) {
	o, err := s.repo.Get(ctx, r.GetId())
	if errors.Is(err, domain.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "order %d not found", r.GetId())
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "lookup failed")
	}
	return toProto(o), nil
}

func toProto(o domain.Order) *ordersv1.Order {
	items := make([]*ordersv1.OrderItem, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, &ordersv1.OrderItem{
			ProductId:  it.ProductID,
			Quantity:   it.Quantity,
			Name:       it.Name,
			PriceCents: it.PriceCents,
		})
	}
	return &ordersv1.Order{Id: o.ID, Status: o.Status, TotalCents: o.TotalCents, Items: items}
}
