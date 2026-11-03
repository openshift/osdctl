package utils

import (
	"io"
	"testing"
)

func TestNewDiscardLogger(t *testing.T) {
	logger := NewDiscardLogger()

	if logger.Out != io.Discard {
		t.Fatal("logger output is not discarded")
	}
}
