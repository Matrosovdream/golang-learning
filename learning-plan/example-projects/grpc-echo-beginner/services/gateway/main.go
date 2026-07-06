// The gateway: the only public service. It speaks REST to the outside world and
// gRPC to the echo service. It mints a request-id at the HTTP edge; the client
// interceptor forwards that id to echo, so both services log the same id.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"grpcecho/pkg/obs"
	echov1 "grpcecho/proto/echo/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func main() {
	logger := obs.NewLogger("gateway")
	httpAddr := ":" + env("HTTP_PORT", "8080")
	echoAddr := env("ECHO_ADDR", "localhost:50051")

	// One long-lived client connection to echo. NewClient is lazy — it won't
	// fail here just because echo isn't up yet; the first RPC finds out.
	conn, err := grpc.NewClient(echoAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(obs.RequestIDUnaryClient),
	)
	if err != nil {
		log.Fatalf("grpc client: %v", err)
	}
	defer conn.Close()
	echo := echov1.NewEchoServiceClient(conn)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /echo/{msg}", func(w http.ResponseWriter, r *http.Request) {
		// Reuse an inbound request-id if the caller sent one, else mint one.
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = obs.NewID()
		}
		ctx := obs.WithRequestID(r.Context(), reqID)
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second) // always bound the call
		defer cancel()

		reply, err := echo.Echo(ctx, &echov1.EchoRequest{Message: r.PathValue("msg")})
		logger.Info("http_request",
			"path", r.URL.Path,
			"downstream_code", status.Code(err).String(),
			"request_id", reqID,
		)
		w.Header().Set("X-Request-Id", reqID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"message":     reply.GetMessage(),
			"served_by":   reply.GetServedBy(),
			"received_at": reply.GetReceivedAt(),
			"request_id":  reqID,
		})
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{Addr: httpAddr, Handler: mux}

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		logger.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	logger.Info("listening", "http", httpAddr, "echo", echoAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

// writeError maps a gRPC status code onto an HTTP status — the gateway's whole job.
func writeError(w http.ResponseWriter, err error) {
	code := status.Code(err)
	httpStatus := map[codes.Code]int{
		codes.InvalidArgument:  http.StatusBadRequest,
		codes.NotFound:         http.StatusNotFound,
		codes.AlreadyExists:    http.StatusConflict,
		codes.Unavailable:      http.StatusServiceUnavailable, // echo is down
		codes.DeadlineExceeded: http.StatusGatewayTimeout,
	}[code]
	if httpStatus == 0 {
		httpStatus = http.StatusInternalServerError
	}
	writeJSON(w, httpStatus, map[string]any{"error": status.Convert(err).Message(), "code": code.String()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
