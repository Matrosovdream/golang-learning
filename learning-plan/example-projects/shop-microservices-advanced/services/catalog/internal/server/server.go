package server

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "shop/proto/catalog/v1"
	"shop/services/catalog/internal/domain"
)

// Server implements the CatalogService gRPC API.
type Server struct {
	catalogv1.UnimplementedCatalogServiceServer
	repo domain.Repository
}

func New(repo domain.Repository) *Server {
	return &Server{repo: repo}
}

func (s *Server) CreateProduct(ctx context.Context, req *catalogv1.CreateProductRequest) (*catalogv1.Product, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetPriceCents() < 0 {
		return nil, status.Error(codes.InvalidArgument, "price_cents must be >= 0")
	}
	if req.GetStock() < 0 {
		return nil, status.Error(codes.InvalidArgument, "stock must be >= 0")
	}
	p := &domain.Product{Name: name, PriceCents: req.GetPriceCents(), Stock: int(req.GetStock())}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, status.Error(codes.Internal, "could not create product")
	}
	return toProto(p), nil
}

func (s *Server) GetProduct(ctx context.Context, req *catalogv1.GetProductRequest) (*catalogv1.Product, error) {
	p, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			return nil, status.Error(codes.NotFound, "product not found")
		}
		return nil, status.Error(codes.Internal, "could not get product")
	}
	return toProto(p), nil
}

func (s *Server) ListProducts(ctx context.Context, _ *catalogv1.ListProductsRequest) (*catalogv1.ListProductsResponse, error) {
	products, err := s.repo.List(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not list products")
	}
	resp := &catalogv1.ListProductsResponse{Products: make([]*catalogv1.Product, len(products))}
	for i := range products {
		resp.Products[i] = toProto(&products[i])
	}
	return resp, nil
}

func (s *Server) ReserveStock(ctx context.Context, req *catalogv1.ReserveStockRequest) (*catalogv1.ReserveStockResponse, error) {
	if len(req.GetItems()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one item is required")
	}
	lines := make([]domain.StockLine, len(req.GetItems()))
	for i, it := range req.GetItems() {
		if it.GetQuantity() <= 0 {
			return nil, status.Error(codes.InvalidArgument, "quantity must be > 0")
		}
		lines[i] = domain.StockLine{ProductID: it.GetProductId(), Quantity: int(it.GetQuantity())}
	}

	reserved, err := s.repo.Reserve(ctx, lines)
	if err != nil {
		var stockErr domain.InsufficientStockError
		switch {
		case errors.As(err, &stockErr):
			return nil, status.Error(codes.FailedPrecondition, stockErr.Error())
		case errors.Is(err, domain.ErrProductNotFound):
			return nil, status.Error(codes.NotFound, "product not found")
		default:
			return nil, status.Error(codes.Internal, "could not reserve stock")
		}
	}

	resp := &catalogv1.ReserveStockResponse{Items: make([]*catalogv1.ReservedItem, len(reserved))}
	for i, l := range reserved {
		resp.Items[i] = &catalogv1.ReservedItem{
			ProductId:      l.ProductID,
			ProductName:    l.ProductName,
			Quantity:       int32(l.Quantity),
			UnitPriceCents: l.UnitPriceCents,
		}
	}
	return resp, nil
}

func toProto(p *domain.Product) *catalogv1.Product {
	return &catalogv1.Product{
		Id:         p.ID,
		Name:       p.Name,
		PriceCents: p.PriceCents,
		Stock:      int32(p.Stock),
		CreatedAt:  p.CreatedAt.Format(time.RFC3339),
	}
}
