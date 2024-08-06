package internal_test

import (
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/dustinspecker/linux-job-worker-service/internal"
)

func TestOutputBufferWrite(t *testing.T) {
	t.Parallel()

	ob := internal.NewOutput()

	bytesWritten, err := ob.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Expected no error invoking Write, got %v", err)
	}

	expectedBytesWritten := 5 // length of "hello"
	if bytesWritten != expectedBytesWritten {
		t.Errorf("Expected %d bytes written, got %d", expectedBytesWritten, bytesWritten)
	}
}

func TestOutputBufferWriteErrorsWhenClosed(t *testing.T) {
	t.Parallel()

	output := internal.NewOutput()

	err := output.Close()
	if err != nil {
		t.Fatalf("Expected no error calling Close, got %v", err)
	}

	_, err = output.Write([]byte("test"))
	if err == nil {
		t.Fatalf("Expected error calling Write, got nil")
	}
}

func TestOutputBufferReadPartial(t *testing.T) {
	t.Parallel()

	output := internal.NewOutput()

	for range 3 {
		_, err := output.Write([]byte("hello"))
		if err != nil {
			t.Fatalf("Expected no error calling Write, got %v", err)
		}
	}

	err := output.Close()
	if err != nil {
		t.Fatalf("Expected no error calling Close, got %v", err)
	}

	// can read entire content
	buffer := make([]byte, 15)

	bytesRead, err := output.ReadPartial(buffer, 0)
	if err == nil {
		t.Fatal("Expected an EOF error to be returned when all content is read, got nil")
	}

	if !errors.Is(err, io.EOF) {
		t.Fatalf("Expected io.EOF, got %v", err)
	}

	if bytesRead != 15 {
		t.Errorf("Expected 15 bytes read, got %d", bytesRead)
	}

	expectedOutput := "hellohellohello"
	if string(buffer) != expectedOutput {
		t.Errorf("Expected output content to be %q, got %q", expectedOutput, string(buffer))
	}

	// can read partial content and starting at offset
	buffer = make([]byte, 5)

	bytesRead, err = output.ReadPartial(buffer, 2)
	if err != nil {
		t.Fatalf("Expected no error calling ReadPartial, got %v", err)
	}

	if bytesRead != 5 {
		t.Errorf("Expected 5 bytes read, got %d", bytesRead)
	}

	expectedOutput = "llohe"
	if string(buffer) != expectedOutput {
		t.Errorf("Expected partial output to be %q, got %q", expectedOutput, string(buffer))
	}
}

func TestReadPartialReturnsEOFWhenClosed(t *testing.T) {
	t.Parallel()

	output := internal.NewOutput()

	_, err := output.Write([]byte("test"))
	if err != nil {
		t.Fatalf("Expected no error calling Write, got %v", err)
	}

	err = output.Close()
	if err != nil {
		t.Fatalf("Expected no error calling Close, got %v", err)
	}

	// validate buffer large enough to contain content gets EOF
	buffer := make([]byte, 5)

	bytesRead, err := output.ReadPartial(buffer, 0)
	if err == nil {
		t.Fatalf("Expected error calling ReadPartial, got nil")
	}

	if bytesRead != 4 {
		t.Errorf("Expected 4 bytes read to read all of test, got %d", bytesRead)
	}

	if string(buffer[0:bytesRead]) != "test" {
		t.Errorf("Expected buffer to include 'test', got %q", buffer[0:bytesRead])
	}

	// validate small buffer does not get EOF when not reading all content
	smallBuffer := make([]byte, 2)

	bytesRead, err = output.ReadPartial(smallBuffer, 0)
	if err != nil {
		t.Fatalf("Expected no error calling ReadPartial, got %v", err)
	}

	if bytesRead != 2 {
		t.Errorf("Expected 2 bytes read to read part of test, got %d", bytesRead)
	}

	if string(smallBuffer[0:bytesRead]) != "te" {
		t.Errorf("Expected smallBuffer to include 'te', got %q", smallBuffer[0:bytesRead])
	}
}

func TestConcurrentReadWrites(t *testing.T) {
	// This test case is to help verify a mutex is being used when race detector is enabled for test cases using -race.
	// TODO: call t.Skip if -race is enabled
	t.Parallel()

	output := internal.NewOutput()

	var writeGroups sync.WaitGroup
	for range 10 {
		writeGroups.Add(1)

		go func() {
			defer writeGroups.Done()

			_, err := output.Write([]byte("hello"))
			if err != nil {
				t.Errorf("Expected no error calling Write, got %v", err)
			}
		}()
	}

	var readGroups sync.WaitGroup
	for range 10 {
		readGroups.Add(1)

		go func() {
			defer readGroups.Done()

			buffer := make([]byte, 1)

			_, err := output.ReadPartial(buffer, 0)
			if err != nil {
				t.Errorf("Expected no error calling ReadPartial, got %v", err)
			}
		}()
	}

	writeGroups.Wait()

	if err := output.Close(); err != nil {
		t.Fatalf("Expected no error calling Close, got %v", err)
	}

	readGroups.Wait()
}

func TestConcurrentWrites(t *testing.T) {
	t.Parallel()

	output := internal.NewOutput()

	var waitGroup sync.WaitGroup
	for range 10 {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			_, err := output.Write([]byte("hello"))
			if err != nil {
				t.Errorf("Expected no error calling Write, got %v", err)
			}
		}()
	}

	for range 10 {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			_, err := output.Write([]byte("world"))
			if err != nil {
				t.Errorf("Expected no error calling Write, got %v", err)
			}
		}()
	}

	waitGroup.Wait()

	if err := output.Close(); err != nil {
		t.Fatalf("Expected no error calling Close, got %v", err)
	}

	buffer := make([]byte, 101)

	bytesRead, err := output.ReadPartial(buffer, 0)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Expected EOF, but got %v", err)
	}

	if bytesRead != 100 {
		t.Errorf("Expected all 100 bytes read, got %d", bytesRead)
	}
}
