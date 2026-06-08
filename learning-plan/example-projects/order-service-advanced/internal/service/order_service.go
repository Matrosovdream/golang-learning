package service

import (
	"context"

	"orderservice/internal/domain"
)

// OrderService holds the order business rules. The transactional heavy lifting
// (locking, stock checks, state-machine enforcement) lives in the repository;
// the service validates input and names the lifecycle transitions.
type OrderService struct {
	repo domain.OrderRepository
}

func NewOrderService(repo domain.OrderRepository) *OrderService {
	return &OrderService{repo: repo}
}

// Checkout validates the requested lines, merges duplicate products, and asks
// the repository to create the order atomically.
func (s *OrderService) Checkout(ctx context.Context, lines []domain.OrderLine) (*domain.Order, error) {
	if len(lines) == 0 {
		return nil, domain.ValidationError{Field: "items", Message: "at least one item is required"}
	}
	merged := map[int64]int{}
	var order []int64
	for _, l := range lines {
		if l.ProductID <= 0 {
			return nil, domain.ValidationError{Field: "product_id", Message: "is invalid"}
		}
		if l.Quantity <= 0 {
			return nil, domain.ValidationError{Field: "quantity", Message: "must be > 0"}
		}
		if _, seen := merged[l.ProductID]; !seen {
			order = append(order, l.ProductID)
		}
		merged[l.ProductID] += l.Quantity
	}
	out := make([]domain.OrderLine, 0, len(order))
	for _, pid := range order {
		out = append(out, domain.OrderLine{ProductID: pid, Quantity: merged[pid]})
	}
	return s.repo.CreateOrder(ctx, out)
}

func (s *OrderService) Get(ctx context.Context, id int64) (*domain.Order, error) {
	return s.repo.GetOrder(ctx, id)
}

func (s *OrderService) List(ctx context.Context) ([]domain.Order, error) {
	return s.repo.ListOrders(ctx)
}

func (s *OrderService) Pay(ctx context.Context, id int64) (*domain.Order, error) {
	return s.repo.SetOrderStatus(ctx, id, domain.StatusPaid)
}

func (s *OrderService) Ship(ctx context.Context, id int64) (*domain.Order, error) {
	return s.repo.SetOrderStatus(ctx, id, domain.StatusShipped)
}

func (s *OrderService) Cancel(ctx context.Context, id int64) (*domain.Order, error) {
	return s.repo.SetOrderStatus(ctx, id, domain.StatusCancelled)
}
