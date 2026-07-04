package apply

import (
	"errors"
	"testing"
)

func TestSystemctlMissingUnit(t *testing.T) {
	for _, errText := range []string{
		"exit status 5: Unit chronyd.service not loaded.",
		"exit status 5: Unit chronyd.service not found.",
		"exit status 1: Failed to disable unit: Unit file chronyd.service does not exist.",
	} {
		if !systemctlMissingUnit(errors.New(errText)) {
			t.Fatalf("missing unit not detected for %q", errText)
		}
	}
	if systemctlMissingUnit(errors.New("exit status 1: access denied")) {
		t.Fatal("access denied classified as missing unit")
	}
}
