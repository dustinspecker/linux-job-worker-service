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
// Output implements io.Closer and io.Writer.
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

// ReadPartial copies content from the Output to buffer starting at the given offset.
// ReadPartial is similar to io.ReaderAt but ReadPartial does not block if less bytes are available than requested.
func (ob *Output) ReadPartial(buffer []byte, off int64) (int, error) {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	content := ob.content

	if off > int64(len(content)) {
		return 0, ErrOffsetOutsideContentBounds
	}

	bytesCopied := copy(buffer, content[off:])

	if bytesCopied+int(off) == len(content) && ob.isClosed {
		return bytesCopied, io.EOF
	}

	return bytesCopied, nil
}

// Wait blocks until new content is written to the Output or the Output is closed.
func (ob *Output) Wait(nextByteIndex int64) {
	ob.mutex.RLock()

	closed := ob.isClosed
	contentLength := int64(len(ob.content))

	ob.mutex.RUnlock()

	// only wait for changes if the output is open or the content contains the next byte to read already
	if closed || contentLength > nextByteIndex {
		return
	}

	ob.mutex.Lock()
	for !ob.isClosed && nextByteIndex >= int64(len(ob.content)) {
		ob.waitCondition.Wait()
	}
	ob.mutex.Unlock()
}

// Close closes the Output preventing any further writes.
func (ob *Output) Close() error {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	ob.isClosed = true

	ob.waitCondition.Broadcast()

	return nil
}
