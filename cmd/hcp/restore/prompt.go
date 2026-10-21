package restore

import (
	"fmt"
	"io"
	"strings"
)

func confirm(in io.Reader, out io.Writer, message string) bool {
	fmt.Fprint(out, message)

	var response string
	if _, err := fmt.Fscanln(in, &response); err != nil {
		return false
	}
	return strings.EqualFold(response, "y") || strings.EqualFold(response, "yes")
}
