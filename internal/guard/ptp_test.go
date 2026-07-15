package guard

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alexzhang1030/time-sync-cli/internal/status"
)

type fakeRunner struct {
	commands []string
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	f.commands = append(f.commands, strings.Join(append([]string{name}, args...), " "))
	return nil, nil
}

type fakeRTCWriter struct {
	writes []time.Time
}

func (f *fakeRTCWriter) WriteSystemTime(t time.Time) error {
	f.writes = append(f.writes, t)
	return nil
}

func configuredPTPClientReport(r *status.Report) *status.Report {
	r.ConfiguredRole = "client"
	r.ConfiguredPTP = true
	return r
}

func TestPTPOnceStartsPHC2SysWhenHealthy(t *testing.T) {
	runner := &fakeRunner{}
	result, err := PTPOnce(Options{
		Runner: runner,
		Collect: func() (*status.Report, error) {
			return configuredPTPClientReport(&status.Report{
				PTPHealth:   "true",
				ClockHealth: "true",
				PTP: status.PTPStatus{
					PHC2SysActive: false,
					PortState:     "SLAVE",
					MasterOffset:  "42",
				},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "start phc2sys" {
		t.Fatalf("Action = %q, want start phc2sys", result.Action)
	}
	want := []string{"systemctl start phc2sys"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestPTPOnceStartsPHC2SysWhenPTPHealthyAndClockUnhealthy(t *testing.T) {
	runner := &fakeRunner{}
	result, err := PTPOnce(Options{
		Runner: runner,
		Collect: func() (*status.Report, error) {
			return configuredPTPClientReport(&status.Report{
				PTPHealth:   "true",
				ClockHealth: "false",
				PTP: status.PTPStatus{
					PHC2SysActive: false,
					PortState:     "SLAVE",
					MasterOffset:  "42",
				},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "start phc2sys" {
		t.Fatalf("Action = %q, want start phc2sys", result.Action)
	}
	want := []string{"systemctl start phc2sys"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestPTPOnceSyncsRTCWhenPTPAndSystemClockAreTrusted(t *testing.T) {
	runner := &fakeRunner{}
	rtc := &fakeRTCWriter{}
	residual := int64(0)
	result, err := PTPOnce(Options{
		Runner:    runner,
		RTCWriter: rtc,
		Collect: func() (*status.Report, error) {
			return configuredPTPClientReport(&status.Report{
				PTPHealth:   "true",
				ClockHealth: "false",
				PTP: status.PTPStatus{
					PHC2SysActive: true,
					PortState:     "SLAVE",
					MasterOffset:  "42",
				},
				Clock: status.ClockStatus{
					SystemUnix:    1783162152,
					RTCUnix:       1783150000,
					PHCUnix:       1783162189,
					PHCResidualNS: &residual,
				},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "sync rtc" {
		t.Fatalf("Action = %q, want sync rtc", result.Action)
	}
	if len(rtc.writes) != 1 || rtc.writes[0].Unix() != 1783162152 {
		t.Fatalf("rtc writes = %#v, want system unix 1783162152", rtc.writes)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want none", runner.commands)
	}
}

func TestPTPOnceSkipsRTCSyncWhenNormalizedPHCResidualIsLarge(t *testing.T) {
	rtc := &fakeRTCWriter{}
	residual := int64(2 * time.Second)
	result, err := PTPOnce(Options{
		Runner:    &fakeRunner{},
		RTCWriter: rtc,
		Collect: func() (*status.Report, error) {
			return configuredPTPClientReport(&status.Report{
				PTPHealth: "true",
				PTP: status.PTPStatus{
					PHC2SysActive: true,
					PortState:     "SLAVE",
					MasterOffset:  "42",
				},
				Clock: status.ClockStatus{
					SystemUnix:    1783162152,
					RTCUnix:       1783150000,
					PHCUnix:       1783162189,
					PHCResidualNS: &residual,
				},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "none" || len(rtc.writes) != 0 {
		t.Fatalf("result = %+v writes=%v", result, rtc.writes)
	}
}

func TestPTPOnceSkipsRTCSyncWhenSystemAndPHCDiffer(t *testing.T) {
	rtc := &fakeRTCWriter{}
	result, err := PTPOnce(Options{
		Runner:    &fakeRunner{},
		RTCWriter: rtc,
		Collect: func() (*status.Report, error) {
			return configuredPTPClientReport(&status.Report{
				PTPHealth:   "true",
				ClockHealth: "false",
				PTP: status.PTPStatus{
					PHC2SysActive: true,
					PortState:     "SLAVE",
					MasterOffset:  "42",
				},
				Clock: status.ClockStatus{
					SystemUnix: 1038,
					RTCUnix:    1783150000,
					PHCUnix:    1783162189,
				},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "none" {
		t.Fatalf("Action = %q, want none", result.Action)
	}
	if len(rtc.writes) != 0 {
		t.Fatalf("rtc writes = %#v, want none", rtc.writes)
	}
}

func TestPTPOnceStopsPHC2SysWhenUnhealthy(t *testing.T) {
	runner := &fakeRunner{}
	result, err := PTPOnce(Options{
		Runner: runner,
		Collect: func() (*status.Report, error) {
			return configuredPTPClientReport(&status.Report{
				PTPHealth:      "false",
				ClockHealth:    "true",
				ConfiguredRole: "client",
				PTP: status.PTPStatus{
					PHC2SysActive: true,
					PortState:     "MASTER",
					MasterOffset:  "42",
				},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "stop phc2sys" {
		t.Fatalf("Action = %q, want stop phc2sys", result.Action)
	}
	want := []string{"systemctl stop phc2sys"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestPTPOnceHoldsPHC2SysStoppedWhenUnhealthy(t *testing.T) {
	runner := &fakeRunner{}
	result, err := PTPOnce(Options{
		Runner: runner,
		Collect: func() (*status.Report, error) {
			return configuredPTPClientReport(&status.Report{
				PTPHealth:   "unknown",
				ClockHealth: "true",
				PTP: status.PTPStatus{
					PHC2SysActive: false,
				},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "hold phc2sys stopped" {
		t.Fatalf("Action = %q, want hold phc2sys stopped", result.Action)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want none", runner.commands)
	}
}

func TestPTPOnceStopsPHC2SysOutsideConfiguredPTPRole(t *testing.T) {
	runner := &fakeRunner{}
	result, err := PTPOnce(Options{
		Runner: runner,
		Collect: func() (*status.Report, error) {
			return &status.Report{
				ConfiguredRole: "auto",
				ConfiguredPTP:  true,
				PTPHealth:      "true",
				ClockHealth:    "true",
				PTP: status.PTPStatus{
					PHC2SysActive: true,
					PortState:     "SLAVE",
					MasterOffset:  "42",
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "stop phc2sys outside configured ptp role" {
		t.Fatalf("Action = %q, want stop phc2sys outside configured ptp role", result.Action)
	}
	want := []string{"systemctl stop phc2sys"}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestPTPOnceHoldsPHC2SysStoppedOutsideConfiguredPTPRole(t *testing.T) {
	runner := &fakeRunner{}
	result, err := PTPOnce(Options{
		Runner: runner,
		Collect: func() (*status.Report, error) {
			return &status.Report{
				ConfiguredRole: "client",
				ConfiguredPTP:  false,
				PTPHealth:      "true",
				ClockHealth:    "true",
				PTP: status.PTPStatus{
					PHC2SysActive: false,
					PortState:     "SLAVE",
					MasterOffset:  "42",
				},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "hold phc2sys stopped outside configured ptp role" {
		t.Fatalf("Action = %q, want hold phc2sys stopped outside configured ptp role", result.Action)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want none", runner.commands)
	}
}

func TestPTPOnceReturnsCollectError(t *testing.T) {
	_, err := PTPOnce(Options{
		Runner: &fakeRunner{},
		Collect: func() (*status.Report, error) {
			return nil, errors.New("collect failed")
		},
	})
	if err == nil {
		t.Fatal("expected collect error")
	}
}
