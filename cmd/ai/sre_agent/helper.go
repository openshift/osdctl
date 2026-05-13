package sreagent

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// copyRepository copies a directory recursively
func copyRepository(sourcePath, destPath string) error {
	fmt.Fprintf(os.Stderr, "Copying repository to %s...\n", destPath)
	cmd := exec.Command("cp", "-r", sourcePath, destPath)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

// promptUserInput reads a line of user input from stdin
func promptUserInput() string {
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.ToLower(strings.TrimSpace(input))
}
