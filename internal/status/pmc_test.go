package status

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPMCQueryArgsUsesWritableClientSocket(t *testing.T) {
	args, socket := pmcQueryArgs("PORT_DATA_SET")
	if filepath.Dir(socket) != filepath.Clean(os.TempDir()) {
		t.Fatalf("socket dir = %q, want %q", filepath.Dir(socket), os.TempDir())
	}
	if filepath.Ext(socket) != ".sock" {
		t.Fatalf("socket = %q, want .sock suffix", socket)
	}

	want := []string{"-u", "-i", socket, "-b", "0", "GET", "PORT_DATA_SET"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}
}

func TestPMCQueryArgsWithServerSocket(t *testing.T) {
	args, socket := pmcQueryArgsWithServer("PORT_DATA_SET", "/var/run/ptp4l")
	want := []string{"-u", "-s", "/var/run/ptp4l", "-i", socket, "-b", "0", "GET", "PORT_DATA_SET"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}
}
