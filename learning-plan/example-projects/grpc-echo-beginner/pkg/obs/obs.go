// Package obs holds the shared observability plumbing: a JSON slog logger and
// the interceptors that give every request a correlation id (request-id) and one
// structured log line. Both services import it, so a single request-id ties the
// gateway's log line to the echo service's log line.
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mdKey is the metadata/header key the request-id travels under.
const mdKey = "x-request-id"

// ctxKey is an unexported type so our context value can't collide with anyone
// else's — the standard idiom for context keys.
type ctxKey struct{}

// NewLogger returns a JSON logger that stamps every line with the service name.
func NewLogger(service string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service)
}

// NewID mints a short random correlation id (used at the HTTP edge).
func NewID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // rand.Read never fails on a healthy system
	return hex.EncodeToString(b[:])
}

// WithRequestID stores an id in ctx. The gateway calls this in its HTTP handler.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestID reads the id back out of ctx (or "none" if unset).
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return "none"
}

// RequestIDUnaryServer reads x-request-id from the incoming metadata (minting one
// if the caller didn't send it) and puts it in the context.
func RequestIDUnaryServer(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	id := ""
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get(mdKey); len(v) > 0 {
			id = v[0]
		}
	}
	if id == "" {
		id = NewID()
	}
	return handler(WithRequestID(ctx, id), req)
}

// RequestIDUnaryClient re-attaches the context's request-id to every outgoing
// call, so the id survives the hop to the next service.
func RequestIDUnaryClient(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx = metadata.AppendToOutgoingContext(ctx, mdKey, RequestID(ctx))
	return invoker(ctx, method, req, reply, cc, opts...)
}

// LoggingUnaryServer logs exactly one structured line per RPC: method, resulting
// status code, latency, and the request-id that ties services together.
func LoggingUnaryServer(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		logger.Info("grpc_request",
			"method", info.FullMethod,
			"code", status.Code(err).String(),
			"ms", time.Since(start).Milliseconds(),
			"request_id", RequestID(ctx),
		)
		return resp, err
	}
}
