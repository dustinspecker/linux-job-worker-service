package main

import (
	"flag"
	"log"

	"github.com/dustinspecker/linux-job-worker-service/internal/client"
)

func main() {
	caCertPath := flag.String("ca-cert-path", "", "path to CA certificate")
	clientCertPath := flag.String("client-cert-path", "", "path to client certificate")
	clientKeyPath := flag.String("client-key-path", "", "path to client certificate")

	flag.Parse()

	if *caCertPath == "" || *clientCertPath == "" || *clientKeyPath == "" {
		log.Fatal("ca-cert-path, client-cert-path, and client-key-path options are required")
	}

	if err := client.Run(*caCertPath, *clientCertPath, *clientKeyPath, flag.Args()); err != nil {
		log.Fatalf("error: %v", err)
	}
}
