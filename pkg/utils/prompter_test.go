package utils

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockingReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingReader) Read(_ []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return 0, io.EOF
}

func TestDefaultPrompterPrompt(t *testing.T) {
	t.Run("writes a prompt and returns trimmed input", func(t *testing.T) {
		var output bytes.Buffer
		prompter := NewDefaultPrompter(
			WithPromptReader(bufio.NewReader(strings.NewReader("  answer  \n"))),
			WithPromptWriter(&output),
		)

		value, err := prompter.Prompt(context.Background(), "Enter value: ")
		if err != nil {
			t.Fatalf("Prompt() error = %v", err)
		}
		if value != "answer" {
			t.Errorf("Prompt() value = %q, want %q", value, "answer")
		}
		if output.String() != "Enter value: " {
			t.Errorf("Prompt() output = %q, want %q", output.String(), "Enter value: ")
		}
	})

	t.Run("returns cancellation while input is blocked", func(t *testing.T) {
		reader := &blockingReader{started: make(chan struct{}), release: make(chan struct{})}
		defer close(reader.release)
		prompter := NewDefaultPrompter(WithPromptReader(bufio.NewReader(reader)), WithPromptWriter(io.Discard))

		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := prompter.Prompt(ctx, "Enter value: ")
			result <- err
		}()

		<-reader.started
		cancel()

		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Prompt() error = %v, want context.Canceled", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Prompt() did not return after cancellation")
		}
	})
}
