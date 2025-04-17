package org

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckOrgId(t *testing.T) {
	tests := []struct {
		Name          string
		Args          []string
		ErrorExpected bool
	}{
		{
			Name:          "Org Id provided",
			Args:          []string{"testOrgId"},
			ErrorExpected: false,
		},
		{
			Name:          "No Org Id provided",
			Args:          []string{},
			ErrorExpected: true,
		},
		{
			Name:          "Multiple Org id provided",
			Args:          []string{"testOrgId1", "testOrgId2"},
			ErrorExpected: true,
		},
	}

	for _, test := range tests {
		err := checkOrgId(test.Args)
		if test.ErrorExpected {
			if err == nil {
				t.Fatalf("Test '%s' failed. Expected error, but got none", test.Name)
			}
		} else {
			if err != nil {
				t.Fatalf("Test '%s' failed. Expected no error, but got '%v'", test.Name, err)
			}
		}
	}
}

func TestPrintOrgTable(t *testing.T) {
	org := Organization{
		ID:         "123",
		Name:       "Example Org",
		ExternalID: "ext123",
		Created:    "2023-01-01T00:00:00Z",
		Updated:    "2023-01-02T00:00:00Z",
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	printOrg(org)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 6)

	// Use regex to check each line, accounting for variable whitespace
	assert.Regexp(t, regexp.MustCompile(`^ID:\s+123$`), lines[0])
	assert.Regexp(t, regexp.MustCompile(`^Name:\s+Example Org$`), lines[1])
	assert.Regexp(t, regexp.MustCompile(`^External ID:\s+ext123$`), lines[2])
	assert.Regexp(t, regexp.MustCompile(`^Created:\s+2023-01-01T00:00:00Z$`), lines[4])
	assert.Regexp(t, regexp.MustCompile(`^Updated:\s+2023-01-02T00:00:00Z$`), lines[5])
}
