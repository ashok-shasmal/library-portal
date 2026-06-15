package main

import (
	"log"
	"net"

	"google.golang.org/grpc"

	"github.com/ashok-shasmal/library-portal/internal/pb"
	"github.com/ashok-shasmal/library-portal/payment/service"
)

func main() {
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterPaymentServiceServer(grpcServer, service.NewPaymentServer())
	log.Println("payment service listening on :50051")
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("payment service failed: %v", err)
	}
}
