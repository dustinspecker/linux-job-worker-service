package internal_test

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/dustinspecker/linux-job-worker-service/internal"
)

func TestOutputReaderReadReturnsFullContent(t *testing.T) {
	t.Parallel()

	output := internal.NewOutput()

	_, err := output.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Expected no error writing content, got %v", err)
	}

	outputReader := internal.NewOutputReader(output)

	// read returns content from the start
	buffer := make([]byte, 4)

	bytesRead, err := outputReader.Read(buffer)
	if err != nil {
		t.Fatalf("Expected no error reading initial content, got %v", err)
	}

	expectedBytesRead := 4
	if bytesRead != expectedBytesRead {
		t.Errorf("Expected %d bytes read, got %d", expectedBytesRead, bytesRead)
	}

	expectedContent := "hell"
	if string(buffer) != expectedContent {
		t.Errorf("Expected %q, got %q", expectedContent, string(buffer))
	}

	// invoking read again returns the next chunk of data
	_, err = output.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Expected no error writing more content, got %v", err)
	}

	_, err = outputReader.Read(buffer)
	if err != nil {
		t.Fatalf("Expected no error reading again, got %v", err)
	}

	if string(buffer) != "ohel" {
		t.Errorf("Expected %q since Read should continue reading from next byte, got %q", "ohel", string(buffer))
	}

	// reading the rest of the content
	err = output.Close()
	if err != nil {
		t.Fatalf("Expected no error closing output, got %v", err)
	}

	bytesRead, err = outputReader.Read(buffer)
	if err == nil {
		t.Fatalf("Expected an error when end of content is reached, but got nil")
	}

	if !errors.Is(err, io.EOF) {
		t.Errorf("Expected %q when end of content is reached, got %q", io.EOF, err)
	}

	if string(buffer[0:bytesRead]) != "lo" {
		t.Errorf("Expected %q since Read should return rest of the content, got %q", "lo", string(buffer))
	}
}

func TestOutputReaderGetsNewContentAsWritten(t *testing.T) {
	t.Parallel()

	output := internal.NewOutput()

	var waitGroup sync.WaitGroup

	// multiple readers should get the same content
	for range 3 {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			outputReader := internal.NewOutputReader(output)

			var chunksRead []string

			for {
				buffer := make([]byte, 10)

				bytesRead, err := outputReader.Read(buffer)
				if err != nil && !errors.Is(err, io.EOF) {
					t.Errorf("Expected no error reading content, got %v", err)
				}

				if bytesRead == 0 && !errors.Is(err, io.EOF) {
					// bytesRead should only be zero if no bytes are available to read
					// and output is closed
					t.Error("Expected EOF when zero bytes read. Read should not return with zero bytes read otherwise.")
				}

				chunksRead = append(chunksRead, string(buffer[0:bytesRead]))

				if errors.Is(err, io.EOF) {
					break
				}
			}

			// validate 4 chunks are returned by read
			// hello
			// world
			// test
			// (empty) EOF
			if len(chunksRead) == 4 {
				if chunksRead[0] != "hello" || chunksRead[1] != "world" || chunksRead[2] != "test" || chunksRead[3] != "" {
					t.Errorf("Expected %q, %q, %q, %q, got %q, %q, %q, %q", "hello", "world", "test", "", chunksRead[0], chunksRead[1], chunksRead[2], chunksRead[3])
				}
			} else {
				t.Errorf("Expected 4 chunks read, got %d", len(chunksRead))
			}
		}()
	}

	waitGroup.Add(1)

	go func() {
		defer waitGroup.Done()

		for _, content := range []string{"hello", "world", "test"} {
			_, err := output.Write([]byte(content))
			if err != nil {
				t.Errorf("Expected no error writing content, got %v", err)
			}

			// sleep to prove readers get content as it is written
			time.Sleep(10 * time.Millisecond)
		}

		err := output.Close()
		if err != nil {
			t.Errorf("Expected no error closing output, got %v", err)
		}
	}()

	waitGroup.Wait()
}
