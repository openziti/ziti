package tests

// ConfigSet describes a named collection of config files for a single test scenario.
// All file paths are relative to the tests/ working directory.
type ConfigSet struct {
	Name           string // display name matching the testdata/configs subdirectory
	CtrlConfig     string // controller config file; empty if the set does not define one
	EdgeRouter     string // edge router config; empty if the set does not define one
	TunnelerRouter string // tunneler-enabled edge router config; empty if not defined
	TransitRouter  string // transit router config; empty if not defined
}

// DefaultATS is the standard full-stack config set used by the majority of the
// integration test suite. It covers the controller, edge router, tunneler router,
// and transit router.
var DefaultATS = ConfigSet{
	Name:           "default-ats",
	CtrlConfig:     "ats-ctrl.yml",
	EdgeRouter:     "ats-edge.router.yml",
	TunnelerRouter: "ats-edge-tunneler.router.yml",
	TransitRouter:  "ats-transit.router.yml",
}
