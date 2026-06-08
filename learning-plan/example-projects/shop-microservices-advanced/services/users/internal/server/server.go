package server

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	usersv1 "shop/proto/users/v1"
	"shop/services/users/internal/domain"
)

// Server implements the UsersService gRPC API by mapping proto messages to and
// from the domain and translating domain errors into gRPC status codes.
type Server struct {
	usersv1.UnimplementedUsersServiceServer
	repo domain.Repository
}

func New(repo domain.Repository) *Server {
	return &Server{repo: repo}
}

func (s *Server) CreateUser(ctx context.Context, req *usersv1.CreateUserRequest) (*usersv1.User, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(req.GetEmail()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "email is invalid")
	}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	u := &domain.User{Email: strings.ToLower(addr.Address), Name: name}
	if err := s.repo.Create(ctx, u); err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}
		return nil, status.Error(codes.Internal, "could not create user")
	}
	return toProto(u), nil
}

func (s *Server) GetUser(ctx context.Context, req *usersv1.GetUserRequest) (*usersv1.User, error) {
	u, err := s.repo.GetByID(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "could not get user")
	}
	return toProto(u), nil
}

func toProto(u *domain.User) *usersv1.User {
	return &usersv1.User{
		Id:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt.Format(time.RFC3339),
	}
}
