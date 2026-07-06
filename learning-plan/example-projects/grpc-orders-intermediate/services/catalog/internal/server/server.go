// Package server is the gRPC transport for catalog: it implements the generated
// CatalogServiceServer, translating between protobuf messages and the domain, and
// mapping domain errors to gRPC status codes.
package server

import (
	"context"
	"errors"

	catalogv1 "grpcorders/proto/catalog/v1"
	"grpcorders/services/catalog/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	catalogv1.UnimplementedCatalogServiceServer // forward-compat embed
	repo                                        domain.ProductRepository
}

func New(repo domain.ProductRepository) *Server { return &Server{repo: repo} }

func (s *Server) CreateProduct(ctx context.Context, r *catalogv1.CreateProductRequest) (*catalogv1.Product, error) {
	if r.GetName() == "" || r.GetPriceCents() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "name and positive price_cents are required")
	}
	p, err := s.repo.Create(ctx, domain.Product{Name: r.GetName(), PriceCents: r.GetPriceCents(), Stock: r.GetStock()})
	if err != nil {
		return nil, status.Error(codes.Internal, "could not create product")
	}
	return toProto(p), nil
}

func (s *Server) GetProduct(ctx context.Context, r *catalogv1.GetProductRequest) (*catalogv1.Product, error) {
	p, err := s.repo.Get(ctx, r.GetId())
	if errors.Is(err, domain.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "product %d not found", r.GetId())
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "lookup failed")
	}
	return toProto(p), nil
}

func (s *Server) ListProducts(ctx context.Context, _ *catalogv1.ListProductsRequest) (*catalogv1.ListProductsReply, error) {
	ps, err := s.repo.List(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "list failed")
	}
	out := make([]*catalogv1.Product, 0, len(ps))
	for _, p := range ps {
		out = append(out, toProto(p))
	}
	return &catalogv1.ListProductsReply{Products: out}, nil
}

func (s *Server) ReserveStock(ctx context.Context, r *catalogv1.ReserveStockRequest) (*catalogv1.ReserveStockReply, error) {
	if r.GetQuantity() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity must be positive")
	}
	p, err := s.repo.Reserve(ctx, r.GetProductId(), r.GetQuantity())
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return nil, status.Errorf(codes.NotFound, "product %d not found", r.GetProductId())
	case errors.Is(err, domain.ErrInsufficientStock):
		// FailedPrecondition: the request was valid but the state won't allow it.
		return nil, status.Errorf(codes.FailedPrecondition, "insufficient stock for product %d", r.GetProductId())
	case err != nil:
		return nil, status.Error(codes.Internal, "reserve failed")
	}
	return &catalogv1.ReserveStockReply{Name: p.Name, PriceCents: p.PriceCents}, nil
}

func toProto(p domain.Product) *catalogv1.Product {
	return &catalogv1.Product{Id: p.ID, Name: p.Name, PriceCents: p.PriceCents, Stock: p.Stock}
}
