package resize

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPromptGenerateResizeSL(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedError error
	}{
		{
			name:          "User cancels service log generation",
			input:         "n\n",
			expectedError: nil,
		},
		{
			name:          "Failed to search for clusters",
			input:         "y\nJIRA-123\njustification text\n",
			expectedError: errors.New("failed to search for clusters with provided filters"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr, pw, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stdin = pr
			go func() {
				pw.Write([]byte(tt.input))
				pw.Close()
			}()

			err = promptGenerateResizeSL("cluster-123", "new-instance-type")
			if tt.expectedError != nil {
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
