package utils

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Prompter reads interactive input while allowing cancellation.
type Prompter interface {
	Prompt(ctx context.Context, message string) (string, error)
}

// DefaultPrompter prompts using a buffered reader and writer.
type DefaultPrompter struct {
	reader *bufio.Reader
	writer io.Writer
}

var _ Prompter = (*DefaultPrompter)(nil)

// PrompterOption configures a DefaultPrompter.
type PrompterOption func(*DefaultPrompter)

// WithPromptReader sets the input reader.
func WithPromptReader(reader io.Reader) PrompterOption {
	return func(prompter *DefaultPrompter) {
		if reader != nil {
			prompter.reader = bufio.NewReader(reader)
		}
	}
}

// WithPromptWriter sets the prompt output writer.
func WithPromptWriter(writer io.Writer) PrompterOption {
	return func(prompter *DefaultPrompter) {
		if writer != nil {
			prompter.writer = writer
		}
	}
}

// NewDefaultPrompter creates a prompter using standard input and error by default.
func NewDefaultPrompter(opts ...PrompterOption) *DefaultPrompter {
	prompter := &DefaultPrompter{
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stderr,
	}
	for _, opt := range opts {
		opt(prompter)
	}
	return prompter
}

// Prompt writes a message and returns the next trimmed line of input.
func (p *DefaultPrompter) Prompt(ctx context.Context, message string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := fmt.Fprint(p.writer, message); err != nil {
		return "", fmt.Errorf("writing prompt: %w", err)
	}

	type readResult struct {
		value string
		err   error
	}

	result := make(chan readResult, 1)
	go func() {
		value, err := p.reader.ReadString('\n')
		result <- readResult{value: value, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case read := <-result:
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if read.err != nil && !errors.Is(read.err, io.EOF) {
			return "", fmt.Errorf("reading input: %w", read.err)
		}
		return strings.TrimSpace(read.value), nil
	}
}
