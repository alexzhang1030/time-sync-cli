package model

// Role defines the time sync operating mode.
type Role string

const (
	RoleAuto   Role = "auto"
	RoleMaster Role = "master"
	RoleClient Role = "client"
)

// ApplyOptions captures flags for apply commands.
type ApplyOptions struct {
	Role         Role
	Iface        string
	NTPPool      string
	NTPServeCIDR string
	Source       string
	PTP          bool
	DryRun       bool
	Yes          bool
}

// PlannedChange describes a single change the planner would apply.
type PlannedChange struct {
	Kind        string `json:"kind" yaml:"kind"`
	Path        string `json:"path,omitempty" yaml:"path,omitempty"`
	Description string `json:"description" yaml:"description"`
	Content     string `json:"content,omitempty" yaml:"content,omitempty"`
}

// Plan is the output of dry-run planning.
type Plan struct {
	Role         Role            `json:"role" yaml:"role"`
	Iface        string          `json:"iface" yaml:"iface"`
	PTP          bool            `json:"ptp" yaml:"ptp"`
	Changes      []PlannedChange `json:"changes" yaml:"changes"`
	DisableUnits []string        `json:"disable_units,omitempty" yaml:"disable_units,omitempty"`
	Warnings     []string        `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}
