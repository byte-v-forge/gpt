package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/byte-v-forge/gpt/gopay/paymentsvc"
	"github.com/byte-v-forge/gpt/gopay/pb"
	"google.golang.org/grpc"
)

func main() {
	cfg := paymentsvc.ConfigFromEnv()
	listen := flag.String("listen", cfg.ListenAddr, "PaymentService listen address")
	flag.Parse()
	server := grpc.NewServer()
	service := paymentsvc.NewServer(cfg)
	pb.RegisterPaymentServiceServer(server, service)
	listener, err := net.Listen("tcp", paymentsvc.NormalizeListenForMain(*listen))
	if err != nil {
		log.Fatalf("listen payment service: %v", err)
	}
	fmt.Printf("[gopay-payment] Go gRPC listening on %s\n", listener.Addr().String())
	if err := server.Serve(listener); err != nil {
		log.Fatalf("serve payment service: %v", err)
	}
}
