// Package grpcserve runs a gRPC server with graceful shutdown.
package grpcserve

import (
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
)

// Serve listens on port and serves srv until SIGINT/SIGTERM, then stops gracefully.
func Serve(logger *slog.Logger, name, port string, srv *grpc.Server) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		logger.Info("shutting down", "service", name)
		srv.GracefulStop()
	}()
	logger.Info("grpc listening", "service", name, "port", port)
	return srv.Serve(lis)
}
