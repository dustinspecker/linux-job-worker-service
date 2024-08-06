package integration_test

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TODO: create helper functions to run command and return output and error
// or consider using gexec: https://onsi.github.io/gomega/#codegexeccode-testing-external-processes

// TODO: add tests proving resource limits are used by client/server

func TestStartJobAndGetStatusAndOutput(t *testing.T) {
	t.Parallel()

	// start
	startStdout := bytes.NewBuffer(nil)
	startStderr := bytes.NewBuffer(nil)
	startCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-1-cert.pem", "-client-key-path=../../certs/client-1-key.pem", "start", "-cpu=1", "-memory=1000000000", "-io=10000000", "echo", "hello")
	startCmd.Stdout = startStdout
	startCmd.Stderr = startStderr

	if err := startCmd.Run(); err != nil {
		t.Logf("stdout: %s", startStdout.String())
		t.Logf("stderr: %s", startStderr.String())
		t.Fatalf("unexpected error running start command: %v", err)
	}

	jobID := strings.TrimSpace(strings.Split(startStdout.String(), ":")[1])

	// status
	statusStdout := bytes.NewBuffer(nil)
	statusStderr := bytes.NewBuffer(nil)
	statusCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-1-cert.pem", "-client-key-path=../../certs/client-1-key.pem", "status", jobID)
	statusCmd.Stdout = statusStdout
	statusCmd.Stderr = statusStderr

	if err := statusCmd.Run(); err != nil {
		t.Logf("stdout: %s", statusStdout.String())
		t.Logf("stderr: %s", statusStderr.String())
		t.Fatalf("unexpected error running status command: %v", err)
	}

	expectedStatus := "status: completed\nexit code: 0\nexit reason: \n"
	if expectedStatus != statusStdout.String() {
		t.Fatalf("expected status to be %s, got %s", expectedStatus, statusStdout.String())
	}

	// stream
	streamStdout := bytes.NewBuffer(nil)
	streamStderr := bytes.NewBuffer(nil)
	streamCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-1-cert.pem", "-client-key-path=../../certs/client-1-key.pem", "stream", jobID)
	streamCmd.Stdout = streamStdout
	streamCmd.Stderr = streamStderr

	if err := streamCmd.Run(); err != nil {
		t.Logf("stdout: %s", streamStdout.String())
		t.Logf("stderr: %s", streamStderr.String())
		t.Fatalf("unexpected error running stream command: %v", err)
	}

	expectedstream := "hello\n"
	if expectedstream != streamStdout.String() {
		t.Fatalf("expected stream to be %s, got %s", expectedstream, streamStdout.String())
	}
}

func TestStopLongLivedJob(t *testing.T) {
	t.Parallel()

	// start
	startStdout := bytes.NewBuffer(nil)
	startStderr := bytes.NewBuffer(nil)
	startCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-1-cert.pem", "-client-key-path=../../certs/client-1-key.pem", "start", "-cpu=1", "-memory=1000000000", "-io=10000000", "sleep", "infinity")
	startCmd.Stdout = startStdout
	startCmd.Stderr = startStderr

	if err := startCmd.Run(); err != nil {
		t.Logf("stdout: %s", startStdout.String())
		t.Logf("stderr: %s", startStderr.String())
		t.Fatalf("unexpected error running start command: %v", err)
	}

	jobID := strings.TrimSpace(strings.Split(startStdout.String(), ":")[1])

	// stop
	stopStdout := bytes.NewBuffer(nil)
	stopStderr := bytes.NewBuffer(nil)
	stopCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-1-cert.pem", "-client-key-path=../../certs/client-1-key.pem", "stop", jobID)
	stopCmd.Stdout = stopStdout
	stopCmd.Stderr = stopStderr

	if err := stopCmd.Run(); err != nil {
		t.Logf("stdout: %s", stopStdout.String())
		t.Logf("stderr: %s", stopStderr.String())
		t.Fatalf("unexpected error running stop command: %v", err)
	}

	// status
	statusStdout := bytes.NewBuffer(nil)
	statusStderr := bytes.NewBuffer(nil)
	statusCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-1-cert.pem", "-client-key-path=../../certs/client-1-key.pem", "status", jobID)
	statusCmd.Stdout = statusStdout
	statusCmd.Stderr = statusStderr

	if err := statusCmd.Run(); err != nil {
		t.Logf("stdout: %s", statusStdout.String())
		t.Logf("stderr: %s", statusStderr.String())
		t.Fatalf("unexpected error running status command: %v", err)
	}

	expectedStatus := "status: terminated\nexit code: -1\nexit reason: \n"
	if expectedStatus != statusStdout.String() {
		t.Fatalf("expected status to be %s, got %s", expectedStatus, statusStdout.String())
	}
}

func TestUsersMayNotAccessJobsNotStartedByThem(t *testing.T) {
	t.Parallel()

	// start job by client-1
	startStdout := bytes.NewBuffer(nil)
	startStderr := bytes.NewBuffer(nil)
	startCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-1-cert.pem", "-client-key-path=../../certs/client-1-key.pem", "start", "-cpu=1", "-memory=1000000000", "-io=10000000", "sleep", "infinity")
	startCmd.Stdout = startStdout
	startCmd.Stderr = startStderr

	if err := startCmd.Run(); err != nil {
		t.Logf("stdout: %s", startStdout.String())
		t.Logf("stderr: %s", startStderr.String())
		t.Fatalf("unexpected error running start command: %v", err)
	}

	jobID := strings.TrimSpace(strings.Split(startStdout.String(), ":")[1])

	// stop as client-2
	stopStdout := bytes.NewBuffer(nil)
	stopStderr := bytes.NewBuffer(nil)
	stopCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-2-cert.pem", "-client-key-path=../../certs/client-2-key.pem", "stop", jobID)
	stopCmd.Stdout = stopStdout
	stopCmd.Stderr = stopStderr

	if err := stopCmd.Run(); err == nil {
		t.Logf("stdout: %s", stopStdout.String())
		t.Logf("stderr: %s", stopStderr.String())
		t.Fatal("expected user to be blocked from stopping a job they did not start")
	}

	if !strings.Contains(stopStderr.String(), "user is not authorized") {
		t.Fatalf("expected error message to contain 'user is not authorized', got %s", stopStderr.String())
	}

	// status as client-2
	statusStdout := bytes.NewBuffer(nil)
	statusStderr := bytes.NewBuffer(nil)
	statusCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-2-cert.pem", "-client-key-path=../../certs/client-2-key.pem", "status", jobID)
	statusCmd.Stdout = statusStdout
	statusCmd.Stderr = statusStderr

	if err := statusCmd.Run(); err == nil {
		t.Logf("stdout: %s", statusStdout.String())
		t.Logf("stderr: %s", statusStderr.String())
		t.Fatal("expected user to be blocked from getting status of a job they did not start")
	}

	if !strings.Contains(statusStderr.String(), "user is not authorized") {
		t.Fatalf("expected error message to contain 'user is not authorized', got %s", statusStderr.String())
	}

	// status as client-2
	streamStdout := bytes.NewBuffer(nil)
	streamStderr := bytes.NewBuffer(nil)
	streamCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-2-cert.pem", "-client-key-path=../../certs/client-2-key.pem", "stream", jobID)
	streamCmd.Stdout = streamStdout
	streamCmd.Stderr = streamStderr

	if err := streamCmd.Run(); err == nil {
		t.Logf("stdout: %s", streamStdout.String())
		t.Logf("stderr: %s", streamStderr.String())
		t.Fatal("expected user to be blocked from streaming a job they did not start")
	}

	if !strings.Contains(streamStderr.String(), "user is not authorized") {
		t.Fatalf("expected error message to contain 'user is not authorized', got %s", streamStderr.String())
	}
}

func TestMultipleClientsCanStreamConcurrently(t *testing.T) {
	// TODO: this should really be a benchmark test
	// This test case attempts to demonstrate that multiple clients can stream concurrently from the same job in
	// a performant manner.
	// This test case spins up 1000 clients each streaming the output to assert that each message is received within
	// 100ms.
	// TODO: this test case could assert average and mean times.
	t.Parallel()

	// start job
	startStdout := bytes.NewBuffer(nil)
	startStderr := bytes.NewBuffer(nil)
	// command will do the following:
	// sleep for 10 seconds (to allow time for all 1000 clients to spin up)
	// print the current unix time in milliseconds every second for 10 times
	// each client should receive a message every second
	// When the client writes, the current time can be saved with each chunk.
	// Once the program is done, the client can check that the time received is within 100ms of the time the message was written by the job.
	startCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-1-cert.pem", "-client-key-path=../../certs/client-1-key.pem", "start", "-cpu=1", "-memory=1000000000", "-io=10000000", "/bin/bash", "-c", "sleep 10; for ((i=0; i<10; i++)); do date +%s%3N; sleep 1; done")
	startCmd.Stdout = startStdout
	startCmd.Stderr = startStderr

	if err := startCmd.Run(); err != nil {
		t.Logf("stdout: %s", startStdout.String())
		t.Logf("stderr: %s", startStderr.String())
		t.Fatalf("unexpected error running start command: %v", err)
	}

	jobID := strings.TrimSpace(strings.Split(startStdout.String(), ":")[1])

	var streamGroup sync.WaitGroup
	for range 1000 {
		streamGroup.Add(1)

		go func() {
			streamStdout := &testWriter{}
			streamCmd := exec.Command("../../bin/client", "-ca-cert-path=../../certs/ca-cert.pem", "-client-cert-path=../../certs/client-1-cert.pem", "-client-key-path=../../certs/client-1-key.pem", "stream", jobID)
			streamCmd.Stdout = streamStdout

			if err := streamCmd.Run(); err != nil {
				t.Errorf("unexpected error running stream command: %v", err)
			}

			for _, chunk := range streamStdout.chunks {
				messageOutputTime, err := strconv.Atoi(strings.TrimSpace(chunk.message))
				if err != nil {
					t.Errorf("unexpected error converting message to int: %v", err)
				}

				timeSpent := chunk.messageArrivalTimeInMilliseconds - int64(messageOutputTime)
				if timeSpent > 100 {
					t.Errorf("expected message to be received within 100ms, but got %d milliseconds", timeSpent)
				}
			}

			defer streamGroup.Done()
		}()
	}

	streamGroup.Wait()
}

type testMessage struct {
	messageArrivalTimeInMilliseconds int64
	message                          string
}

type testWriter struct {
	chunks []testMessage
}

func (w *testWriter) Write(content []byte) (int, error) {
	now := time.Now().UnixMilli()

	w.chunks = append(w.chunks, testMessage{
		messageArrivalTimeInMilliseconds: now,
		message:                          string(content),
	})

	return len(content), nil
}
