package apply

import (
	"fmt"

	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

// ValidatePTPHardware ensures the target interface supports hardware timestamping when PTP is requested.
func ValidatePTPHardware(opts model.ApplyOptions) error {
	if !opts.PTP {
		return nil
	}
	iface := opts.Iface
	if iface == "" {
		iface = "eth0"
	}
	ok, err := detect.InterfaceHardwareTS(iface)
	if err != nil {
		return fmt.Errorf("PTP hardware check on %s: %w", iface, err)
	}
	if !ok {
		return fmt.Errorf("interface %s lacks PTP hardware timestamping; run timesync doctor", iface)
	}
	return nil
}
