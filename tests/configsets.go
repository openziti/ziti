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
// not explicitly present in the configuration.
var NoExplicitOIDC = ConfigSet{
	Name:       "no-explicit-oidc",
	CtrlConfig: "testdata/configs/no-explicit-oidc/ctrl.yml",
}

// DisabledOidcAutoBinding is a controller-only config set with the edge-oidc binding
// omitted from the web listener AND disableOidcAutoBinding set to true. Used to verify
// that the auto-binding behaviour is suppressed when the operator opts out, leaving OIDC
// absent from the running controller.
var DisabledOidcAutoBinding = ConfigSet{
	Name:       "disabled-oidc-auto-binding",
	CtrlConfig: "testdata/configs/disabled-oidc-auto-binding/ctrl.yml",
}

// DualOidcServers is a controller-only config set with two web server entries, each
// on a different port and each hosting the edge-oidc API. Used to verify that the OIDC
// discovery document returns issuer-specific endpoint URLs that reflect the port the
// client connected to.
var DualOidcServers = ConfigSet{
	Name:       "dual-oidc-servers",
	CtrlConfig: "testdata/configs/dual-oidc-servers/ctrl.yml",
}
