package apply

import (
	"errors"
	"strings"
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

func TestValidatePTPHardwareSkipsChecksWhenPTPDisabled(t *testing.T) {
	restore := stubPTPValidation(t, nil, func(string) (bool, error) {
		t.Fatal("hardware check should not run")
		return false, nil
	})
	defer restore()

	if err := ValidatePTPHardware(model.ApplyOptions{PTP: false}); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePTPHardwareRejectsMissingPTPBinaries(t *testing.T) {
	restore := stubPTPValidation(t, map[string]bool{
		"ptp4l":   true,
		"phc2sys": true,
		"phc_ctl": false,
		"pmc":     false,
	}, func(string) (bool, error) {
		t.Fatal("hardware check should not run when binaries are missing")
		return false, nil
	})
	defer restore()

	err := ValidatePTPHardware(model.ApplyOptions{PTP: true, Iface: "eth0"})
	if err == nil {
		t.Fatal("expected missing binary error")
	}
	if !strings.Contains(err.Error(), "phc_ctl") || !strings.Contains(err.Error(), "pmc") {
		t.Fatalf("error = %q, want missing phc_ctl and pmc", err)
	}
}

func TestValidatePTPHardwareChecksHardwareWhenBinariesExist(t *testing.T) {
	restore := stubPTPValidation(t, map[string]bool{
		"ptp4l":   true,
		"phc2sys": true,
		"phc_ctl": true,
		"pmc":     true,
	}, func(iface string) (bool, error) {
		if iface != "eth2" {
			t.Fatalf("iface = %q, want eth2", iface)
		}
		return true, nil
	})
	defer restore()

	if err := ValidatePTPHardware(model.ApplyOptions{PTP: true, Iface: "eth2"}); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePTPHardwareReturnsHardwareError(t *testing.T) {
	restore := stubPTPValidation(t, map[string]bool{
		"ptp4l":   true,
		"phc2sys": true,
		"phc_ctl": true,
		"pmc":     true,
	}, func(string) (bool, error) {
		return false, errors.New("ethtool failed")
	})
	defer restore()

	err := ValidatePTPHardware(model.ApplyOptions{PTP: true, Iface: "eth0"})
	if err == nil || !strings.Contains(err.Error(), "ethtool failed") {
		t.Fatalf("err = %v, want hardware error", err)
	}
}

func stubPTPValidation(t *testing.T, binaries map[string]bool, hardware func(string) (bool, error)) func() {
	t.Helper()
	previousLookPath := lookPath
	previousHardware := interfaceHardwareTS
	lookPath = func(name string) (string, error) {
		if binaries[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("missing")
	}
	interfaceHardwareTS = hardware
	return func() {
		lookPath = previousLookPath
		interfaceHardwareTS = previousHardware
	}
}
