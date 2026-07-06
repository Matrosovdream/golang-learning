// Package obs is the shared observability toolkit every service imports:
//   - a JSON slog logger,
//   - request-id interceptors (server + client) that propagate a correlation id,
//   - a logging interceptor (one structured line per RPC),
//   - a Prometheus metrics interceptor + a side HTTP server that exposes /metrics.
//
// Because all three services use the same package, a request-id set at the gateway
// shows up in every service's logs, and every service exports the same metric names.
package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const mdKey = "x-request-id"

type ctxKey struct{}

// ---- logging + request-id (same as the beginner project) ----

func NewLogger(service string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service)
}

func NewID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return "none"
}

// RequestIDUnaryServer moves the id from incoming metadata into the context.
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

// RequestIDUnaryClient copies the context's id onto every outgoing call, so it
// survives the hop from one service to the next (gateway → orders → catalog).
func RequestIDUnaryClient(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(metadata.AppendToOutgoingContext(ctx, mdKey, RequestID(ctx)), method, req, reply, cc, opts...)
}

// LoggingUnaryServer logs one line per RPC, tagged with the request-id.
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

// ---- metrics ----

// Metrics bundles a service's Prometheus registry and collectors. Each service
// creates one, installs its interceptor, and serves its registry on /metrics.
type Metrics struct {
	reg     *prometheus.Registry
	handled *prometheus.CounterVec
	latency *prometheus.HistogramVec
}

// NewMetrics builds the standard gRPC-server metrics, labelled by this service.
func NewMetrics(service string) *Metrics {
	reg := prometheus.NewRegistry()
	labels := prometheus.Labels{"service": service}

	handled := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_handled_total", Help: "Total RPCs handled, by method and code.",
		ConstLabels: labels,
	}, []string{"method", "code"})

	latency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "grpc_server_handling_seconds", Help: "RPC handling latency in seconds.",
		Buckets: prometheus.DefBuckets, ConstLabels: labels,
	}, []string{"method"})

	// Register our RPC metrics plus the Go runtime + process metrics — free
	// dashboards (goroutines, GC, memory, CPU) for Grafana later.
	reg.MustRegister(handled, latency,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return &Metrics{reg: reg, handled: handled, latency: latency}
}

// UnaryServerInterceptor records a count + latency for every RPC.
func (m *Metrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		m.latency.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())
		m.handled.WithLabelValues(info.FullMethod, status.Code(err).String()).Inc()
		return resp, err
	}
}

// ServeMetrics starts a small HTTP server exposing /metrics (Prometheus scrapes
// this) and /healthz, on a port separate from the gRPC port. Returns the server
// so main can shut it down gracefully.
func ServeMetrics(addr string, m *Metrics, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server", "err", err)
		}
	}()
	logger.Info("metrics listening", "addr", addr)
	return srv
}
