package tests

// ConfigSet describes a named collection of config files for a single test scenario.
// All file paths are relative to the tests/ working directory.
type ConfigSet struct {
	Name           string   // display name matching the testdata/configs subdirectory
	CtrlConfig     string   // controller config file; empty if the set does not define one
	EdgeRouter     string   // edge router config; empty if the set does not define one
	TunnelerRouter string   // tunneler-enabled edge router config; empty if not defined
	TransitRouter  string   // transit router config; empty if not defined
	FabricRouters  []string // fabric-only router configs, ordered by 1-based index
}

// DefaultATS is the standard full-stack config set used by the majority of the
// integration test suite. It covers the controller, edge router, tunneler router,
// transit router, and the fabric-only router pair used by link tests.
var DefaultATS = ConfigSet{
	Name:           "default-ats",
	CtrlConfig:     "testdata/configs/default-ats/ctrl.yml",
	EdgeRouter:     "testdata/configs/default-ats/edge-router.yml",
	TunnelerRouter: "testdata/configs/default-ats/tunneler-router.yml",
	TransitRouter:  "testdata/configs/default-ats/transit-router.yml",
	FabricRouters: []string{
		"testdata/configs/default-ats/fabric-router-1.yml",
		"testdata/configs/default-ats/fabric-router-2.yml",
	},
}

// NoExplicitOIDC is a controller-only config set with the edge-oidc binding omitted
// from the web listener. Used to verify that the controller's
// ensureOidcOnClientApiServer validator automatically adds the OIDC API when it is
// not explicitly present in the configuration (issue #3597).
var NoExplicitOIDC = ConfigSet{
	Name:       "no-explicit-oidc",
	CtrlConfig: "testdata/configs/no-explicit-oidc/ctrl.yml",
}
