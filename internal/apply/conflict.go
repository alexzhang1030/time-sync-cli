package apply

import (
	"os"
	"strings"

	"github.com/alexzhang1030/time-sync-cli/internal/model"
)

// UnmanagedConflicts returns target paths in the plan that already exist on disk
// but were not previously created or managed by timesync. Overwriting these
// would clobber files the tool does not own, so callers should confirm first.
func UnmanagedConflicts(plan *model.Plan) ([]string, error) {
	return DefaultApplier().UnmanagedConflicts(plan)
}

// UnmanagedConflicts inspects the plan against prior state recorded under the
// applier's config dir and the on-disk files.
func (a *Applier) UnmanagedConflicts(plan *model.Plan) ([]string, error) {
	if plan == nil {
		return nil, nil
	}
	managed := a.managedPaths()
	seen := make(map[string]bool)
	var conflicts []string
	for _, change := range plan.Changes {
		if change.Path == "" || seen[change.Path] {
			continue
		}
		seen[change.Path] = true

		info, err := os.Stat(change.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		if managed[change.Path] || hasTimesyncMarker(change.Path) {
			continue
		}
		conflicts = append(conflicts, change.Path)
	}
	return conflicts, nil
}

// managedPaths returns the set of files timesync created or backed up on its
// last apply, as recorded in state.json. A missing/unreadable state yields an
// empty set so every pre-existing target is treated as unmanaged.
func (a *Applier) managedPaths() map[string]bool {
	managed := make(map[string]bool)
	state, err := LoadState(a.ConfigDir)
	if err != nil {
		return managed
	}
	for _, p := range state.Created {
		managed[p] = true
	}
	for p := range state.Backups {
		managed[p] = true
	}
	return managed
}

// hasTimesyncMarker reports whether the file content references timesync-cli,
// which every generated config and systemd drop-in includes.
func hasTimesyncMarker(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "timesync-cli")
}
