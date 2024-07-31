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
// TODO: implement all of ReadAt requirements (return error when bytesCopied < len(buffer)):
// ReadAt isn't as strictly implemented since it is not required for the current use case
// and this code is all unexported.
// https://pkg.go.dev/io#ReaderAt
func (ob *Output) ReadAt(buffer []byte, off int64) (int, error) {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()

	if off > int64(len(ob.content)) {
		return 0, ErrOffsetOutsideContentBounds
	}

	bytesCopied := copy(buffer, ob.content[off:])
	if bytesCopied+int(off) == len(ob.content) && ob.isClosed {
		return bytesCopied, io.EOF
	}

	return bytesCopied, nil
}

// Close closes the Output preventing any further writes.
func (ob *Output) Close() error {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	ob.isClosed = true

	ob.waitCondition.Broadcast()

	return nil
}

// Wait blocks until the content of the Output is different from knownSize
// while the Output is open.
// If Output is closed then Wait will never block.
// If the known size is less than the current size of the Output then Wait will
// return immediately.
// int64 is returned since this is the type used by ReadAt.
func (ob *Output) Wait(knownSize int64) (int64, bool) {
	// use Lock/Unlock since sync.Cond is only aware of Lock/Unlock methods
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	// TODO: handle wait condition being nil
	// If waitCondition is nil then user did not create Output with NewOutput,
	// which shouldn't happen because this code is unexported, so we have control
	// over how Output is created (e.g. job.Job.Start uses NewOutput()).
	for !ob.isClosed && knownSize == int64(len(ob.content)) {
		ob.waitCondition.Wait()
	}

	return int64(len(ob.content)), ob.isClosed
}
