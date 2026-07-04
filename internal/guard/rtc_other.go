//go:build !linux

package guard

import (
	"fmt"
	"time"
)

type linuxRTCWriter struct{}

func (linuxRTCWriter) WriteSystemTime(time.Time) error {
	return fmt.Errorf("RTC writeback requires Linux")
}
