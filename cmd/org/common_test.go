package org

import (
	"testing"
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