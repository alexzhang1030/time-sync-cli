package status_test

import (
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

func TestParseChronyTracking(t *testing.T) {
	line := "^,192.168.1.1,1.2.3.4,1,0.000123,0.000456,0.001"
	source, offset := status.ParseChronyTracking(line)
	if source != "1.2.3.4" {
		t.Errorf("source = %q, want 1.2.3.4", source)
	}
	if offset != "0.000456" {
		t.Errorf("offset = %q, want 0.000456", offset)
	}
}

func TestParseChronyTracking_Empty(t *testing.T) {
	source, offset := status.ParseChronyTracking("")
	if source != "" || offset != "" {
		t.Error("expected empty parse for empty input")
	}
}
