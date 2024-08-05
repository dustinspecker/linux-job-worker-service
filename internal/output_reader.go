package internal

import (
	"errors"
)

var ErrOutputMissing = errors.New("OutputReader's output is nil")

// OutputReader implements io.Reader interface to read from the provided Output.
type OutputReader struct {
	output *Output
	// readIndex is the index of the next byte to read from the Output
	readIndex int64
}

// NewOutputReader creates a new OutputReader instance.
func NewOutputReader(output *Output) *OutputReader {
	return &OutputReader{
		output:    output,
		readIndex: 0,
	}
}

// Read reads from the Output and returns the number of bytes read and an error if any.
// Read returns EOF if the Output is closed and all the content has been read.
func (outputReader *OutputReader) Read(buffer []byte) (int, error) {
	if outputReader.output == nil {
		return 0, ErrOutputMissing
	}

	bytesRead, err := outputReader.output.ReadAt(buffer, outputReader.readIndex)
	if err != nil {
		return bytesRead, err
	}

	outputReader.readIndex += int64(bytesRead)

	return bytesRead, nil
}
