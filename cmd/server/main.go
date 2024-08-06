package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/dustinspecker/linux-job-worker-service/internal/proto"
	"github.com/dustinspecker/linux-job-worker-service/internal/server"
	"github.com/dustinspecker/linux-job-worker-service/pkg/executor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var ErrFailedToAppendCA = errors.New("failed to append CA certificate")

func getServerCreds() (credentials.TransportCredentials, error) {
	// TODO: make paths to server certs configurable via flags
	pemServerCa, err := os.ReadFile("certs/ca-cert.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to load CA certificate: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemServerCa) {
		return nil, ErrFailedToAppendCA
	}

	serverCert, err := tls.LoadX509KeyPair("certs/server-cert.pem", "certs/server-key.pem")
	if err != nil {
		return nil, fmt.Errorf("failed to load server key pair: %w", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
		MinVersion:   tls.VersionTLS13,
	}

	return credentials.NewTLS(config), nil
}

func main() {
	// handle job-executor implementation
	if len(os.Args) > 3 && os.Args[1] == "run" {
		if err := executor.Run(os.Args[2], os.Args[3:]); err != nil {
			log.Fatalf("failed to run job executor: %v", err)
		}

		return
	}

	rootPhysicalDeviceMajMin := flag.String("root-physical", "", "the MAJOR:MIN device ID where root is mounted")
	flag.Parse()

	if *rootPhysicalDeviceMajMin == "" {
		log.Fatal("-root-physical is required")
	}

	// otherwise assume listening for connections
	// TODO: make address to listen on configurable via flag
	lis, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	tlsCreds, err := getServerCreds()
	if err != nil {
		log.Fatalf("failed to get server creds: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.Creds(tlsCreds),
	)
	proto.RegisterJobWorkerServer(grpcServer, server.NewJobWorkerServer(*rootPhysicalDeviceMajMin))
	log.Println("starting server on port 8080")
	log.Fatal("error listening to server:", grpcServer.Serve(lis))
}
