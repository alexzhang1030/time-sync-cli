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

func TestParseEthtoolTimestamping_ModernEtHToolCapabilities(t *testing.T) {
	input := `Time stamping parameters for eth0:
Capabilities:
	hardware-transmit
	software-transmit
	hardware-receive
	software-receive
	software-system-clock
	hardware-raw-clock
PTP Hardware Clock: 0
Hardware Transmit Timestamp Modes:
	off
	on
Hardware Receive Filter Modes:
	none
	all`
	hw, sw := detect.ParseEthtoolTimestamping(input)
	if !hw {
		t.Error("expected hardware timestamping from capabilities")
	}
	if !sw {
		t.Error("expected software timestamping from capabilities")
	}
}

func TestParseEthtoolTimestamping_PTPHardwareClockNone(t *testing.T) {
	input := `PTP Hardware Clock: none`
	hw, _ := detect.ParseEthtoolTimestamping(input)
	if hw {
		t.Error("did not expect hardware timestamping")
	}
}

func TestParseEthtoolTimestamping_PTPHardwareClockNegative(t *testing.T) {
	input := `PTP Hardware Clock: -1`
	hw, _ := detect.ParseEthtoolTimestamping(input)
	if hw {
		t.Error("did not expect hardware timestamping")
	}
}
