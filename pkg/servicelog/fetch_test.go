package servicelog

import (
	"strings"
	"testing"
)

func TestDuplicateWarning_singular(t *testing.T) {
	msg := duplicateWarning(1)
	if !strings.HasPrefix(msg, "1 service log has been sent") {
		t.Errorf("singular warning = %q, want prefix %q", msg, "1 service log has been sent")
	}
	if !strings.Contains(msg, "please verify you are not sending a duplicate") {
		t.Errorf("warning should mention duplicate check, got %q", msg)
	}
}

func TestDuplicateWarning_plural(t *testing.T) {
	msg := duplicateWarning(3)
	if !strings.HasPrefix(msg, "3 service logs have been sent") {
		t.Errorf("plural warning = %q, want prefix %q", msg, "3 service logs have been sent")
	}
	if !strings.Contains(msg, "please verify you are not sending a duplicate") {
		t.Errorf("warning should mention duplicate check, got %q", msg)
	}
}

func TestDuplicateWarning_noDescription(t *testing.T) {
	// Verify that the warning never includes "Description:" — the previous
	// implementation logged each service-log description, which could
	// contain customer-provided data.
	for _, count := range []int{1, 2, 5, 100} {
		msg := duplicateWarning(count)
		if strings.Contains(strings.ToLower(msg), "description") {
			t.Errorf("duplicateWarning(%d) must not reference description, got %q", count, msg)
		}
	}
}

func TestDuplicateWarning_countInMessage(t *testing.T) {
	tests := []struct {
		count    int
		contains string
	}{
		{1, "1 service log"},
		{2, "2 service logs"},
		{10, "10 service logs"},
	}
	for _, tt := range tests {
		msg := duplicateWarning(tt.count)
		if !strings.Contains(msg, tt.contains) {
			t.Errorf("duplicateWarning(%d) = %q, want substring %q", tt.count, msg, tt.contains)
		}
	}
}
