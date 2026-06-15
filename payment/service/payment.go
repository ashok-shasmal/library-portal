package service

import (
	context "context"
	"fmt"

	"github.com/ashok-shasmal/library-portal/internal/pb"
)

type paymentServer struct {
	pb.UnimplementedPaymentServiceServer
}

func NewPaymentServer() pb.PaymentServiceServer {
	return &paymentServer{}
}

func (s *paymentServer) Charge(ctx context.Context, req *pb.PaymentRequest) (*pb.PaymentResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("missing payment request")
	}
	// This the place where the service should connect with payment gateway and
	// respond success or failure
	return &pb.PaymentResponse{
		Success:       true,
		TransactionId: fmt.Sprintf("txn_%d_%d", req.UserId, req.BookId),
		Message:       "payment successful",
	}, nil
}
