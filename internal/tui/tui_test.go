package tui_test

import (
	"strings"
	"testing"

	"github.com/alexzhang1030/time-sync-cli/internal/detect"
	"github.com/alexzhang1030/time-sync-cli/internal/model"
	"github.com/alexzhang1030/time-sync-cli/internal/tui"
)

func TestParseRoleChoice(t *testing.T) {
	tests := []struct {
		in   string
		want model.Role
	}{
		{"1", model.RoleAuto},
		{"auto", model.RoleAuto},
		{"2", model.RoleMaster},
		{"3", model.RoleClient},
	}
	for _, tt := range tests {
		got, err := tui.ParseRoleChoice(tt.in)
		if err != nil {
			t.Fatalf("ParseRoleChoice(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("ParseRoleChoice(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestResolveInterfaceChoice(t *testing.T) {
	ifaces := []detect.InterfaceInfo{
		{Name: "eth0", Up: true},
		{Name: "wlan0", Up: false},
	}
	name, err := tui.ResolveInterfaceChoice("1", ifaces)
	if err != nil || name != "eth0" {
		t.Fatalf("by index: name=%q err=%v", name, err)
	}
	name, err = tui.ResolveInterfaceChoice("wlan0", ifaces)
	if err != nil || name != "wlan0" {
		t.Fatalf("by name: name=%q err=%v", name, err)
	}
}

func TestParseYesNo(t *testing.T) {
	if !tui.ParseYesNo("y", false) {
		t.Error("expected yes")
	}
	if tui.ParseYesNo("n", true) {
		t.Error("expected no")
	}
}

func TestRunWithIO_DryRunAuto(t *testing.T) {
	in := strings.NewReader("1\n\n\n\n\n")
	var out strings.Builder
	err := tui.RunWithIO(in, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Role: auto") {
		t.Error("expected auto plan in output")
	}
	if !strings.Contains(out.String(), "dry-run") {
		t.Error("expected dry-run message")
	}
}
