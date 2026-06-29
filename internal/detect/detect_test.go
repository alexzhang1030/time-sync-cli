package detect_test

import (
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/detect"
)

func TestParseEthtoolTimestamping_Hardware(t *testing.T) {
	input := `Time stamping parameters for eth0:
PTP Hardware Clock: 0
Hardware Transmit Timestamping: on
Hardware Receive Timestamping: on
Software Transmit Timestamping: off
Software Receive Timestamping: off`
	hw, sw := detect.ParseEthtoolTimestamping(input)
	if !hw {
		t.Error("expected hardware timestamping")
	}
	if sw {
		t.Error("did not expect software timestamping")
	}
}

func TestParseEthtoolTimestamping_SoftwareOnly(t *testing.T) {
	input := `Software Transmit Timestamping: on
Software Receive Timestamping: on
Hardware Transmit Timestamping: off`
	hw, sw := detect.ParseEthtoolTimestamping(input)
	if hw {
		t.Error("did not expect hardware timestamping")
	}
	if !sw {
		t.Error("expected software timestamping")
	}
}

func TestParseEthtoolTimestamping_PTPHardwareClock(t *testing.T) {
	input := `PTP Hardware Clock: 1`
	hw, _ := detect.ParseEthtoolTimestamping(input)
	if !hw {
		t.Error("expected hardware from PTP Hardware Clock line")
	}
}
