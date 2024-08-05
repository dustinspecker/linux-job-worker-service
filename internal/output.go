package internal

import (
	"errors"
	"io"
	"sync"
)

var (
	// ErrClosedOutput is returned when attempting to write to a closed Output.
	ErrClosedOutput = errors.New("cannot write to closed Output")

	// ErrOffsetOutsideContentBounds is returned when the offset is greater than the length of the content.
	ErrOffsetOutsideContentBounds = errors.New("offset is greater than the length of the content")
)

// Output is a buffer that can be written to and read from
// by multiple goroutines concurrently.
// Output implements io.Closer, io.ReaderAt, and io.Writer.
// Output should not be created directly, but instead by calling NewOutput.
type Output struct {
	// content store the written data
	content []byte
	// isClosed is true once Close() is called to prevent further writes
	isClosed bool
	// mutex is used to handle concurrent writes and reads
	mutex sync.RWMutex
	// waitCondition is used so goroutines can wait for content to be written or output to be closed
	waitCondition *sync.Cond
}

// NewOutput returns a new Output.
func NewOutput() *Output {
	ob := Output{}
	ob.waitCondition = sync.NewCond(&ob.mutex)

	return &ob
}

// Write appends newContent to the content of the Output.
func (ob *Output) Write(newContent []byte) (int, error) {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	if ob.isClosed {
		return 0, ErrClosedOutput
	}

	ob.content = append(ob.content, newContent...)

	ob.waitCondition.Broadcast()

	return len(newContent), nil
}

// ReadAt copies content from the Output to buffer starting at the given offset.
// Note: as described in https://pkg.go.dev/io#ReaderAt,
// ReadAt will block until the requested bytes are available or the Output is closed.
func (ob *Output) ReadAt(buffer []byte, off int64) (int, error) {
	if off > int64(len(ob.Content())) {
		return 0, ErrOffsetOutsideContentBounds
	}

	// Block until the requested bytes are available or the Output is closed.
	// As part of the io.ReaderAt interface, ReadAt should block until the requested bytes are available.
	for !ob.Closed() && int64(len(ob.Content())) < int64(cap(buffer))+off {
		ob.mutex.Lock()
		ob.waitCondition.Wait()
		ob.mutex.Unlock()
	}

	content := ob.Content()
	bytesCopied := copy(buffer, content[off:])

	if bytesCopied+int(off) == len(content) && ob.Closed() {
		return bytesCopied, io.EOF
	}

	return bytesCopied, nil
}

// Content returns the content of the Output in a thread-safe way.
func (ob *Output) Content() []byte {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	return ob.content
}

// Closed returns true if the Output is closed in a thread-safe way.
func (ob *Output) Closed() bool {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	return ob.isClosed
}

// Close closes the Output preventing any further writes.
func (ob *Output) Close() error {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	ob.isClosed = true

	ob.waitCondition.Broadcast()

	return nil
}
