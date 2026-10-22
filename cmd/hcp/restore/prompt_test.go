package restore

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfirm(t *testing.T) {
	for _, tt := range []struct {
		name, response string
		want           bool
	}{
		{name: "yes", response: "yes\n", want: true},
		{name: "short yes", response: "y\n", want: true},
		{name: "no", response: "no\n", want: false},
		{name: "empty", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder

			assert.Equal(t, tt.want, confirm(strings.NewReader(tt.response), &output, "Continue? [y/N] "))
			assert.Equal(t, "Continue? [y/N] ", output.String())
		})
	}
}
