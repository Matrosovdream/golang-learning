// Package grpcserve runs a gRPC server with graceful shutdown, shared by every
// service that exposes a gRPC API.
package grpcserve

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
)

// Run starts a gRPC server on port, registers services via register, and
// blocks until the server stops or a signal triggers a graceful shutdown.
func Run(name, port string, register func(*grpc.Server)) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	srv := grpc.NewServer()
	register(srv)

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		log.Printf("%s: shutting down", name)
		srv.GracefulStop()
	}()

	log.Printf("%s listening on :%s (gRPC)", name, port)
	return srv.Serve(lis)
}
