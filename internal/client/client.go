package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dustinspecker/linux-job-worker-service/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	ErrFailedToAppendCA    = errors.New("failed to append CA certificate")
	ErrMissingCommand      = errors.New("command is required: start, status, stream, or stop")
	ErrMissingJobID        = errors.New("job id is required")
	ErrStartMissingCommand = errors.New("start requires a command to run")
	ErrStartMissingFlags   = errors.New("start requires -cpu, -memory, and -io flags")
	ErrUnknownCommand      = errors.New("unknown command, allowed commands are: start, status, stream, stop")
)

// Run handles determining which subcommand to run and the arguments required for the subcommands.
func Run(caCertPath string, clientCertPath string, clientKeyPath string, arguments []string) error {
	creds, err := getClientCreds(caCertPath, clientCertPath, clientKeyPath)
	if err != nil {
		return fmt.Errorf("failed to get client creds: %w", err)
	}

	// TODO: make the address configurable
	conn, err := grpc.NewClient("localhost:8080", grpc.WithTransportCredentials(creds))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	client := proto.NewJobWorkerClient(conn)

	if len(arguments) == 0 {
		return ErrMissingCommand
	}

	// TODO/future consideration: use something like cobra to handle subcommands
	switch arguments[0] {
	case "start":
		if len(arguments) < 2 {
			return ErrStartMissingCommand
		}

		// parse flags required only for start command
		startFlagSet := flag.NewFlagSet(arguments[0], flag.ContinueOnError)

		// optional flags that are applicable to start only
		cpu := startFlagSet.Float64("cpu", 0, "number of CPUs to limit the job to")
		memoryInBytes := startFlagSet.Int64("memory", 0, "amount of memory in bytes to limit the job to")
		ioInBytes := startFlagSet.Int64("io", 0, "amount of IO in bytes to limit the job to")

		if err := startFlagSet.Parse(arguments[1:]); err != nil {
			return fmt.Errorf("failed to parse flags: %w", err)
		}

		if *cpu == 0 || *memoryInBytes == 0 || *ioInBytes == 0 {
			return ErrStartMissingFlags
		}

		if err := start(client, startFlagSet.Args()[0], startFlagSet.Args()[1:], *cpu, *memoryInBytes, *ioInBytes); err != nil {
			return fmt.Errorf("failed to start job: %w", err)
		}
	case "status":
		if len(arguments) < 2 {
			return ErrMissingCommand
		}

		if err := status(client, arguments[1]); err != nil {
			return fmt.Errorf("failed to get status: %w", err)
		}
	case "stream":
		if len(arguments) < 2 {
			return ErrMissingJobID
		}

		if err := stream(client, arguments[1]); err != nil {
			return fmt.Errorf("failed to stream job: %w", err)
		}
	case "stop":
		if len(arguments) < 2 {
			return ErrMissingJobID
		}

		if err := stop(client, arguments[1]); err != nil {
			return fmt.Errorf("failed to stop job: %w", err)
		}
	default:
		return ErrUnknownCommand
	}

	return nil
}

func start(client proto.JobWorkerClient, command string, args []string, cpu float64, memory int64, io int64) error {
	job, err := client.Start(context.Background(), &proto.JobStartRequest{
		Cpu:              cpu,
		IoBytesPerSecond: io,
		MemBytes:         memory,
		Command:          command,
		Args:             args,
	})
	if err != nil {
		return fmt.Errorf("failed to start job: %w", err)
	}

	fmt.Println("job id:", job.GetId()) //nolint:forbidigo

	return nil
}

func stream(client proto.JobWorkerClient, jobID string) error {
	job := &proto.Job{
		Id: jobID,
	}

	stream, err := client.Stream(context.Background(), job)
	if err != nil {
		return fmt.Errorf("failed to stream job: %w", err)
	}

	for {
		output, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Print(string(output.GetContent())) //nolint:forbidigo

				break
			}

			return fmt.Errorf("failed to receive output: %w", err)
		}

		fmt.Print(string(output.GetContent())) //nolint:forbidigo
	}

	return nil
}

func status(client proto.JobWorkerClient, jobID string) error {
	job := &proto.Job{
		Id: jobID,
	}

	status, err := client.Query(context.Background(), job)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	fmt.Println("status:", status.GetStatus())          //nolint:forbidigo
	fmt.Println("exit code:", status.GetExitCode())     //nolint:forbidigo
	fmt.Println("exit reason:", status.GetExitReason()) //nolint:forbidigo

	return nil
}

func stop(client proto.JobWorkerClient, jobID string) error {
	job := &proto.Job{
		Id: jobID,
	}

	_, err := client.Stop(context.Background(), job)
	if err != nil {
		return fmt.Errorf("failed to stop job: %w", err)
	}

	return nil
}

func getClientCreds(caCertPath string, clientCertPath string, clientKeyPath string) (credentials.TransportCredentials, error) {
	// TODO: DRY this code up with getServerCreds
	// They both load cert pool and keypair, just different tls config
	pemServerCa, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA certificate: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemServerCa) {
		return nil, ErrFailedToAppendCA
	}

	clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client key pair: %w", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      certPool,
	}

	return credentials.NewTLS(config), nil
}
