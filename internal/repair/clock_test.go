package repair

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	commands []string
	failAt   string
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	command := strings.Join(append([]string{name}, args...), " ")
	f.commands = append(f.commands, command)
	if command == f.failAt {
		return []byte("boom"), errors.New("failed")
	}
	if name == "phc_ctl" && len(args) >= 2 && args[len(args)-1] == "get" {
		t := time.Now().Add(37 * time.Second)
		s := fmt.Sprintf("phc_ctl[1.000]: clock time is %d.%09d or test", t.Unix(), t.Nanosecond())
		return []byte(s), nil
	}
	return nil, nil
}

func TestClockRepairsFromRTCAndStateInterface(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()

	dir := t.TempDir()
	configDir := filepath.Join(dir, "timesync-cli")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "state.json"), []byte(`{"role":"client","iface":"eth0","ptp":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1783162152\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	result, err := Clock(Options{ConfigDir: configDir, RTCPath: rtcPath, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}

	if result.Iface != "eth0" {
		t.Fatalf("Iface = %q, want eth0", result.Iface)
	}
	if result.RTCEpoch != 1783162152 {
		t.Fatalf("RTCEpoch = %d, want 1783162152", result.RTCEpoch)
	}
	want := []string{
		"systemctl stop phc2sys ptp4l",
		"date -u -s @1783162152",
		"phc_ctl eth0 set",
		"systemctl start ptp4l",
		"systemctl start phc2sys",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestClockUsesExplicitInterface(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()

	dir := t.TempDir()
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1783162152\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	result, err := Clock(Options{Iface: "eth2", ConfigDir: filepath.Join(dir, "missing"), RTCPath: rtcPath, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if result.Iface != "eth2" {
		t.Fatalf("Iface = %q, want eth2", result.Iface)
	}
	if runner.commands[2] != "phc_ctl eth2 set" {
		t.Fatalf("phc command = %q, want explicit interface", runner.commands[2])
	}
}

func TestClockRepairsMasterAndPublishesGM(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()

	dir := t.TempDir()
	configDir := filepath.Join(dir, "timesync-cli")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "state.json"), []byte(`{"role":"master","iface":"eth0","ptp":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "ptp4l.conf")
	if err := os.WriteFile(cfgPath, []byte("utc_offset 37\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1783162152\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	publishedIface := ""
	runner := &fakeRunner{}
	result, err := Clock(Options{
		ConfigDir:  configDir,
		ConfigPath: cfgPath,
		RTCPath:    rtcPath,
		Runner:     runner,
		PublishGM: func(iface string) error {
			publishedIface = iface
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Iface != "eth0" {
		t.Fatalf("Iface = %q, want eth0", result.Iface)
	}
	if publishedIface != "eth0" {
		t.Fatalf("published iface = %q, want eth0", publishedIface)
	}

	want := []string{
		"systemctl stop phc2sys ptp4l",
		"date -u -s @1783162152",
		"phc_ctl eth0 set",
		"systemctl start ptp4l",
		"systemctl start phc2sys",
		"phc_ctl eth0 get",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestClockRejectsUntrustedRTC(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()

	dir := t.TempDir()
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1038\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	_, err := Clock(Options{Iface: "eth0", RTCPath: rtcPath, Runner: runner})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want none", runner.commands)
	}
}

func TestBootGuardSeedsEpochSystemClockAndPHC(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()
	restoreNow := stubNow(time.Unix(1038, 0))
	defer restoreNow()

	dir := t.TempDir()
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1783162152\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	result, err := BootGuard(Options{Iface: "eth0", RTCPath: rtcPath, RepairSystemClock: true, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if result.RTCEpoch != 1783162152 {
		t.Fatalf("RTCEpoch = %d, want 1783162152", result.RTCEpoch)
	}
	want := []string{
		"date -u -s @1783162152",
		"phc_ctl eth0 set",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestBootGuardKeepsTrustedSystemClockAndSeedsPHC(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()
	restoreNow := stubNow(time.Unix(1783162152, 0))
	defer restoreNow()

	dir := t.TempDir()
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1783162153\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	result, err := BootGuard(Options{Iface: "eth0", RTCPath: rtcPath, RepairSystemClock: true, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if result.RTCEpoch != 0 {
		t.Fatalf("RTCEpoch = %d, want 0", result.RTCEpoch)
	}
	want := []string{"phc_ctl eth0 set"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestBootGuardKeepsTrustedSystemClockWhenRTCIsEpoch(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()
	restoreNow := stubNow(time.Unix(1783162152, 0))
	defer restoreNow()

	dir := t.TempDir()
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("40\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	result, err := BootGuard(Options{Iface: "eth0", RTCPath: rtcPath, RepairSystemClock: true, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if result.RTCEpoch != 0 {
		t.Fatalf("RTCEpoch = %d, want 0", result.RTCEpoch)
	}
	want := []string{"phc_ctl eth0 set"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestBootGuardRepairModeTrustsSystemClockOverSkewedRTC(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()
	restoreNow := stubNow(time.Unix(1783150000, 0))
	defer restoreNow()

	dir := t.TempDir()
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1783162152\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	result, err := BootGuard(Options{Iface: "eth0", RTCPath: rtcPath, RepairSystemClock: true, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if result.RTCEpoch != 0 {
		t.Fatalf("RTCEpoch = %d, want 0", result.RTCEpoch)
	}
	want := []string{"phc_ctl eth0 set"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestBootGuardRequireTrustedRejectsSystemClockRTCSkew(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()
	restoreNow := stubNow(time.Unix(1783150000, 0))
	defer restoreNow()

	dir := t.TempDir()
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1783162152\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	_, err := BootGuard(Options{Iface: "eth0", RTCPath: rtcPath, RequireTrustedSystemClock: true, Runner: runner})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want none", runner.commands)
	}
}

func TestBootGuardDefaultOnlySeedsPHC(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()
	restoreNow := stubNow(time.Unix(1038, 0))
	defer restoreNow()

	runner := &fakeRunner{}

	result, err := BootGuard(Options{Iface: "eth0", RTCPath: filepath.Join(t.TempDir(), "missing"), Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if result.RTCEpoch != 0 {
		t.Fatalf("RTCEpoch = %d, want 0", result.RTCEpoch)
	}
	want := []string{"phc_ctl eth0 set"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestBootGuardRequiresTrustedSystemClock(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()
	restoreNow := stubNow(time.Unix(1783162152, 0))
	defer restoreNow()

	dir := t.TempDir()
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1783162153\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	_, err := BootGuard(Options{Iface: "eth0", RTCPath: rtcPath, RequireTrustedSystemClock: true, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"phc_ctl eth0 set"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestBootGuardRejectsUntrustedSystemClockWhenRequired(t *testing.T) {
	restoreEUID := stubRoot(t)
	defer restoreEUID()
	restoreNow := stubNow(time.Unix(1038, 0))
	defer restoreNow()

	dir := t.TempDir()
	rtcPath := filepath.Join(dir, "since_epoch")
	if err := os.WriteFile(rtcPath, []byte("1783162152\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	_, err := BootGuard(Options{Iface: "eth0", RTCPath: rtcPath, RequireTrustedSystemClock: true, Runner: runner})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want none", runner.commands)
	}
}

func stubRoot(t *testing.T) func() {
	t.Helper()
	previous := getEUID
	getEUID = func() int { return 0 }
	return func() { getEUID = previous }
}

func stubNow(value time.Time) func() {
	previous := now
	now = func() time.Time { return value }
	return func() { now = previous }
}
