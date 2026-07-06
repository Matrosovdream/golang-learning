// The echo service: a tiny gRPC server. It echoes the message back, tagged with
// the hostname that served it and a timestamp. Two interceptors give it a
// request-id and one structured log line per call.
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"grpcecho/pkg/obs"
	echov1 "grpcecho/proto/echo/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// echoServer implements the generated EchoServiceServer interface. Embedding
// UnimplementedEchoServiceServer keeps it forward-compatible if the proto grows.
type echoServer struct {
	echov1.UnimplementedEchoServiceServer
	hostname string
}

func (s echoServer) Echo(ctx context.Context, r *echov1.EchoRequest) (*echov1.EchoReply, error) {
	return &echov1.EchoReply{
		Message:    r.GetMessage(),
		ServedBy:   s.hostname,
		ReceivedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func main() {
	logger := obs.NewLogger("echo")
	port := env("GRPC_PORT", "50051")
	host, _ := os.Hostname()

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	// Interceptor order matters: request-id first, so the log line already has it.
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		obs.RequestIDUnaryServer,
		obs.LoggingUnaryServer(logger),
	))
	echov1.RegisterEchoServiceServer(srv, echoServer{hostname: host})
	reflection.Register(srv) // lets you poke the server with grpcurl

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		logger.Info("shutting down")
		srv.GracefulStop() // let in-flight RPCs finish
	}()

	logger.Info("listening", "port", port, "host", host)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
