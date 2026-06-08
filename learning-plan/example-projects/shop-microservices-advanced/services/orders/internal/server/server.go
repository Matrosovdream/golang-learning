package server

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	catalogv1 "shop/proto/catalog/v1"
	ordersv1 "shop/proto/orders/v1"
	usersv1 "shop/proto/users/v1"
	"shop/services/orders/internal/domain"
)

// Server implements the OrdersService gRPC API. It depends on the users and
// catalog clients to validate the buyer and reserve/price stock — this is the
// synchronous service-to-service composition at the heart of the project.
type Server struct {
	ordersv1.UnimplementedOrdersServiceServer
	repo    domain.Repository
	users   usersv1.UsersServiceClient
	catalog catalogv1.CatalogServiceClient
}

func New(repo domain.Repository, users usersv1.UsersServiceClient, catalog catalogv1.CatalogServiceClient) *Server {
	return &Server{repo: repo, users: users, catalog: catalog}
}

func (s *Server) CreateOrder(ctx context.Context, req *ordersv1.CreateOrderRequest) (*ordersv1.Order, error) {
	if req.GetUserId() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if len(req.GetItems()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one item is required")
	}

	// 1. Validate the buyer exists (call to the users service).
	if _, err := s.users.GetUser(ctx, &usersv1.GetUserRequest{Id: req.GetUserId()}); err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, status.Error(codes.FailedPrecondition, "user does not exist")
		}
		return nil, status.Error(codes.Internal, "could not validate user")
	}

	// 2. Reserve stock and get prices (call to the catalog service). Its status
	// code (NotFound / FailedPrecondition / InvalidArgument) is forwarded as-is.
	stockItems := make([]*catalogv1.StockItem, len(req.GetItems()))
	for i, it := range req.GetItems() {
		stockItems[i] = &catalogv1.StockItem{ProductId: it.GetProductId(), Quantity: it.GetQuantity()}
	}
	reserved, err := s.catalog.ReserveStock(ctx, &catalogv1.ReserveStockRequest{Items: stockItems})
	if err != nil {
		return nil, err
	}

	// 3. Build and persist the order in this service's own database.
	order := &domain.Order{UserID: req.GetUserId(), Status: "confirmed"}
	for _, it := range reserved.GetItems() {
		order.Items = append(order.Items, domain.OrderItem{
			ProductID:      it.GetProductId(),
			ProductName:    it.GetProductName(),
			Quantity:       int(it.GetQuantity()),
			UnitPriceCents: it.GetUnitPriceCents(),
		})
		order.TotalCents += it.GetUnitPriceCents() * int64(it.GetQuantity())
	}
	if err := s.repo.Create(ctx, order); err != nil {
		return nil, status.Error(codes.Internal, "could not save order")
	}
	return toProto(order), nil
}

func (s *Server) GetOrder(ctx context.Context, req *ordersv1.GetOrderRequest) (*ordersv1.Order, error) {
	o, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Error(codes.Internal, "could not get order")
	}
	return toProto(o), nil
}

func toProto(o *domain.Order) *ordersv1.Order {
	items := make([]*ordersv1.OrderItem, len(o.Items))
	for i, it := range o.Items {
		items[i] = &ordersv1.OrderItem{
			ProductId:      it.ProductID,
			ProductName:    it.ProductName,
			Quantity:       int32(it.Quantity),
			UnitPriceCents: it.UnitPriceCents,
		}
	}
	return &ordersv1.Order{
		Id:         o.ID,
		UserId:     o.UserID,
		Status:     o.Status,
		TotalCents: o.TotalCents,
		Items:      items,
		CreatedAt:  o.CreatedAt.Format(time.RFC3339),
	}
}
