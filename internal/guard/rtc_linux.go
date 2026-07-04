//go:build linux

package guard

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type linuxRTCWriter struct{}

func (linuxRTCWriter) WriteSystemTime(t time.Time) error {
	var lastErr error
	for _, path := range []string{"/dev/rtc0", "/dev/rtc"} {
		file, err := os.OpenFile(path, os.O_RDONLY, 0)
		if err != nil {
			lastErr = err
			continue
		}
		rtc := unix.RTCTime{
			Sec:   int32(t.Second()),
			Min:   int32(t.Minute()),
			Hour:  int32(t.Hour()),
			Mday:  int32(t.Day()),
			Mon:   int32(t.Month()) - 1,
			Year:  int32(t.Year()) - 1900,
			Wday:  int32(t.Weekday()),
			Yday:  int32(t.YearDay()) - 1,
			Isdst: 0,
		}
		err = unix.IoctlSetRTCTime(int(file.Fd()), &rtc)
		closeErr := file.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if closeErr != nil {
			return closeErr
		}
		return nil
	}
	return fmt.Errorf("write RTC: %w", lastErr)
}
