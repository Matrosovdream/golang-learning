package service

import (
	"context"
	"strings"

	"orderservice/internal/domain"
)

// ProductService holds the product business rules.
type ProductService struct {
	repo domain.ProductRepository
}

func NewProductService(repo domain.ProductRepository) *ProductService {
	return &ProductService{repo: repo}
}

// CreateProductInput carries the raw values for a new product.
type CreateProductInput struct {
	SKU        string
	Name       string
	PriceCents int64
	Stock      int
}

func (s *ProductService) Create(ctx context.Context, in CreateProductInput) (*domain.Product, error) {
	sku := strings.TrimSpace(in.SKU)
	if sku == "" {
		return nil, domain.ValidationError{Field: "sku", Message: "is required"}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, domain.ValidationError{Field: "name", Message: "is required"}
	}
	if in.PriceCents < 0 {
		return nil, domain.ValidationError{Field: "price_cents", Message: "must be >= 0"}
	}
	if in.Stock < 0 {
		return nil, domain.ValidationError{Field: "stock", Message: "must be >= 0"}
	}
	p := &domain.Product{SKU: sku, Name: name, PriceCents: in.PriceCents, Stock: in.Stock}
	if err := s.repo.CreateProduct(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *ProductService) Get(ctx context.Context, id int64) (*domain.Product, error) {
	return s.repo.GetProduct(ctx, id)
}

func (s *ProductService) List(ctx context.Context) ([]domain.Product, error) {
	return s.repo.ListProducts(ctx)
}

func (s *ProductService) Restock(ctx context.Context, id int64, qty int) (*domain.Product, error) {
	if qty <= 0 {
		return nil, domain.ValidationError{Field: "quantity", Message: "must be > 0"}
	}
	return s.repo.Restock(ctx, id, qty)
}
