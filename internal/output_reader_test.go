package internal_test

import (
	"errors"
	"io"
	"sync"
	"testing"

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
		t.Errorf("Expected %q since Read should return rest of the content, got %q", "ohel", string(buffer))
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

			fullContent, err := io.ReadAll(outputReader)
			if err != nil {
				t.Errorf("Expected no error reading content, got %v", err)
			}

			expectedContent := "helloworldtest"
			if string(fullContent) != expectedContent {
				t.Errorf("Expected %q, got %q", expectedContent, string(fullContent))
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
		}

		err := output.Close()
		if err != nil {
			t.Errorf("Expected no error closing output, got %v", err)
		}
	}()

	waitGroup.Wait()
}
