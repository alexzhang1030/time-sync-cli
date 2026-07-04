package apply

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

var (
	interfaceHardwareTS = detect.InterfaceHardwareTS
	lookPath            = exec.LookPath
)

// ValidatePTPHardware ensures the target interface supports hardware timestamping when PTP is requested.
func ValidatePTPHardware(opts model.ApplyOptions) error {
	if !opts.PTP {
		return nil
	}
	if missing := missingPTPBinaries(); len(missing) > 0 {
		return fmt.Errorf("missing PTP runtime binaries %s; install linuxptp and re-run timesync doctor", strings.Join(missing, ", "))
	}
	iface := opts.Iface
	if iface == "" {
		iface = "eth0"
	}
	ok, err := interfaceHardwareTS(iface)
	if err != nil {
		return fmt.Errorf("PTP hardware check on %s: %w", iface, err)
	}
	if !ok {
		return fmt.Errorf("interface %s lacks PTP hardware timestamping; run timesync doctor", iface)
	}
	return nil
}

func missingPTPBinaries() []string {
	var missing []string
	for _, name := range []string{"ptp4l", "phc2sys", "phc_ctl", "pmc"} {
		if path, err := lookPath(name); err != nil || path == "" {
			missing = append(missing, name)
		}
	}
	return missing
}
