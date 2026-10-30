package utils

import (
	"io"

	"github.com/sirupsen/logrus"
)

// NewDiscardLogger returns a logger that suppresses all output.
func NewDiscardLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}
