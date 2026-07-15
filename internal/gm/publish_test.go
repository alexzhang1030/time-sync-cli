package gm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type runnerResponse struct {
	out []byte
	err error
}

type fakeRunner struct {
	commands  []string
	responses []runnerResponse
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, strings.Join(append([]string{name}, args...), " "))
	if len(f.responses) == 0 {
		return nil, errors.New("unexpected command")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response.out, response.err
}

func TestPublishPreservesGMQualityAndMarksUTCOffsetValid(t *testing.T) {
	configPath := writeConfig(t, "utc_offset 37\n")
	runner := &fakeRunner{responses: []runnerResponse{
		{out: []byte(phcTime(37 * time.Second))},
		{out: []byte(gmSettings("37", "0"))},
		{out: []byte("RESPONSE MANAGEMENT GRANDMASTER_SETTINGS_NP\n")},
		{out: []byte(gmSettings("37", "1"))},
	}}

	result, err := Publish(Options{
		Runner:     runner,
		ConfigPath: configPath,
		Iface:      "eth0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.UTCOffset != 37 {
		t.Fatalf("UTCOffset = %d, want 37", result.UTCOffset)
	}

	wantSet := "pmc -u -b 0 SET GRANDMASTER_SETTINGS_NP " +
		"clockClass 6 clockAccuracy 0xfe offsetScaledLogVariance 0xffff " +
		"currentUtcOffset 37 leap61 0 leap59 0 currentUtcOffsetValid 1 " +
		"ptpTimescale 1 timeTraceable 0 frequencyTraceable 0 timeSource 0x50"
	want := []string{
		"phc_ctl eth0 get",
		"pmc -u -b 0 GET GRANDMASTER_SETTINGS_NP",
		wantSet,
		"pmc -u -b 0 GET GRANDMASTER_SETTINGS_NP",
	}
	if strings.Join(runner.commands, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands:\n%s\nwant:\n%s", strings.Join(runner.commands, "\n"), strings.Join(want, "\n"))
	}
}

func TestPublishRequiresUTCOffsetInGeneratedConfig(t *testing.T) {
	_, err := Publish(Options{
		Runner:     &fakeRunner{},
		ConfigPath: writeConfig(t, "clockClass 6\n"),
		Iface:      "eth0",
	})
	if err == nil || !strings.Contains(err.Error(), "utc_offset") {
		t.Fatalf("error = %v, want missing utc_offset", err)
	}
}

func TestPublishRejectsUnverifiedSettings(t *testing.T) {
	runner := &fakeRunner{responses: []runnerResponse{
		{out: []byte(phcTime(37 * time.Second))},
		{out: []byte(gmSettings("37", "0"))},
		{out: []byte("RESPONSE MANAGEMENT GRANDMASTER_SETTINGS_NP\n")},
		{out: []byte(gmSettings("37", "0"))},
	}}

	_, err := Publish(Options{
		Runner:     runner,
		ConfigPath: writeConfig(t, "utc_offset 37\n"),
		Iface:      "eth0",
	})
	if err == nil || !strings.Contains(err.Error(), "currentUtcOffsetValid=0") {
		t.Fatalf("error = %v, want verification failure", err)
	}
}

func TestPublishWaitsForPHCToUsePTPTimescale(t *testing.T) {
	runner := &fakeRunner{responses: []runnerResponse{
		{out: []byte(phcTime(0))},
	}}

	_, err := Publish(Options{
		Runner:     runner,
		ConfigPath: writeConfig(t, "utc_offset 37\n"),
		Iface:      "eth0",
	})
	if err == nil || !strings.Contains(err.Error(), "PHC TAI residual") {
		t.Fatalf("error = %v, want PHC readiness failure", err)
	}
	if len(runner.commands) != 1 || runner.commands[0] != "phc_ctl eth0 get" {
		t.Fatalf("commands = %#v, want PHC check only", runner.commands)
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ptp4l.conf")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func gmSettings(offset, valid string) string {
	return `RESPONSE MANAGEMENT GRANDMASTER_SETTINGS_NP
clockClass              6
clockAccuracy           0xfe
offsetScaledLogVariance 0xffff
currentUtcOffset        ` + offset + `
leap61                  0
leap59                  0
currentUtcOffsetValid   ` + valid + `
ptpTimescale            1
timeTraceable           0
frequencyTraceable      0
timeSource              0x50
`
}

func phcTime(offset time.Duration) string {
	value := time.Now().Add(offset)
	return fmt.Sprintf("phc_ctl[1.000]: clock time is %d.%09d or test", value.Unix(), value.Nanosecond())
}
