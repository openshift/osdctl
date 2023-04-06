package cluster

import "testing"

func TestGetClusterNameFromInfraID(t *testing.T) {
	testInfraIdsToName := map[string]string{
		"dev-asdf":           "dev",
		"as-df-ghjkl-qwerty": "as-df-ghjkl",
		"asdf-":              "asdf",
	}
	for infraId, expectedName := range testInfraIdsToName {
		actualName, err := getClusterNameFromInfraId(infraId)
		if err != nil {
			t.Errorf("failed to extract cluster name from infrastructure ID %q", err)
		}
		if actualName != expectedName {
			t.Errorf("expected name, %s, does not match actual name, %s", expectedName, actualName)
		}
	}
}

func TestGetClusterNameFromInfraIdWithOneSegment(t *testing.T) {
	infraIdWithOneSegment := "asdf" // one segment, no hyphens
	actualName, err := getClusterNameFromInfraId(infraIdWithOneSegment)
	if actualName != "" {
		t.Errorf("no name should be returned from infrastructure id missing hyphens, got %s", actualName)
	}
	if err == nil {
		t.Errorf("error should not be nil when passed infrastructure id is missing hyphens")
	}
}

func TestGetClusterNameFromEmptyInfraId(t *testing.T) {
	infraIdWithOneSegment := "" // one segment, no hyphens
	actualName, err := getClusterNameFromInfraId(infraIdWithOneSegment)
	if actualName != "" {
		t.Errorf("no name should be returned from empty infrastructure id, got %s", actualName)
	}
	if err == nil {
		t.Errorf("error should not be nil when passed empty infrastructure id")
	}
}
