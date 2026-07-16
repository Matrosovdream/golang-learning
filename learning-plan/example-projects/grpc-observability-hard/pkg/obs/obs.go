// Package obs is the shared observability toolkit. Same idea as the intermediate
// project, but the metrics set is richer (counter + latency histogram + in-flight
// gauge) so the Grafana dashboard has something to draw: request rate, error
// ratio, p95 latency, and concurrency.
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

func RequestIDUnaryClient(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(metadata.AppendToOutgoingContext(ctx, mdKey, RequestID(ctx)), method, req, reply, cc, opts...)
}

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

// Metrics bundles the Prometheus collectors for one service.
type Metrics struct {
	reg      *prometheus.Registry
	handled  *prometheus.CounterVec
	latency  *prometheus.HistogramVec
	inflight *prometheus.GaugeVec
}

func NewMetrics(service string) *Metrics {
	reg := prometheus.NewRegistry()
	labels := prometheus.Labels{"service": service}

	handled := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_handled_total", Help: "Total RPCs handled, by method and code.",
		ConstLabels: labels,
	}, []string{"method", "code"})

	latency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "grpc_server_handling_seconds", Help: "RPC handling latency in seconds.",
		// buckets tuned for fast in-memory/DB RPCs up to ~2.5s
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
		ConstLabels: labels,
	}, []string{"method"})

	inflight := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "grpc_server_in_flight", Help: "In-flight RPCs.", ConstLabels: labels,
	}, []string{"method"})

	reg.MustRegister(handled, latency, inflight,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return &Metrics{reg: reg, handled: handled, latency: latency, inflight: inflight}
}

func (m *Metrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		m.inflight.WithLabelValues(info.FullMethod).Inc()
		start := time.Now()
		resp, err := handler(ctx, req)
		m.inflight.WithLabelValues(info.FullMethod).Dec()
		m.latency.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())
		m.handled.WithLabelValues(info.FullMethod, status.Code(err).String()).Inc()
		return resp, err
	}
}

func ServeMetrics(addr string, m *Metrics, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server", "err", err)
		}
	}()
	logger.Info("metrics listening", "addr", addr)
	return srv
}
