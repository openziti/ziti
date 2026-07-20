package tests

// ConfigSet describes a named collection of config files for a single test scenario.
// All file paths are relative to the tests/ working directory.
type ConfigSet struct {
	Name            string   // display name matching the testdata/configs subdirectory
	CtrlConfig      string   // controller config file; empty if the set does not define one
	PeerCtrlConfigs []string // additional cluster-member controller configs, joined to the primary by StartHaCluster
	EdgeRouter      string   // edge router config; empty if the set does not define one
	TunnelerRouter  string   // tunneler-enabled edge router config; empty if not defined
	TransitRouter   string   // transit router config; empty if not defined
	FabricRouters   []string // fabric-only router configs, ordered by 1-based index
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

// SingleRaftDataDir is the raft data directory used by the SingleRaft config set. It is cleaned by
// StartServerRaft before each run and must match the cluster.dataDir in single-raft/ctrl.yml.
const SingleRaftDataDir = "testdata/single-raft-data"

// SingleRaft is a controller-only config set that runs a single controller in raft/cluster mode
// (cluster.dataDir set instead of db). Used to exercise the raft self-registration path, where the
// controller records itself in the Controller store on leadership rather than relying on the
// non-raft synthesized-self fallback.
var SingleRaft = ConfigSet{
	Name:       "single-raft",
	CtrlConfig: "testdata/configs/single-raft/ctrl.yml",
}

// Ha3DataDir is the parent raft data directory used by the Ha3 config set. It is cleaned by
// StartHaCluster before each run and must contain the cluster.dataDir of every ha-3 controller.
const Ha3DataDir = "testdata/ha-3-data"

// Ha3 is a three-controller raft cluster config set whose edge signing CA root is distinct from
// the ctrl-channel root CA. Each controller signs identity certs with its own intermediate under
// the shared signing root. Used to exercise first-party client cert validation when the signing
// CA and ctrl-channel CA differ, including certs issued by a controller other than the one a
// router is subscribed to.
var Ha3 = ConfigSet{
	Name:       "ha-3",
	CtrlConfig: "testdata/configs/ha-3/ctrl1.yml",
	PeerCtrlConfigs: []string{
		"testdata/configs/ha-3/ctrl2.yml",
		"testdata/configs/ha-3/ctrl3.yml",
	},
	EdgeRouter: "testdata/configs/ha-3/edge-router.yml",
}

// DualOidcServers is a controller-only config set with two web server entries, each
// on a different port and each hosting the edge-oidc API. Used to verify that the OIDC
// discovery document returns issuer-specific endpoint URLs that reflect the port the
// client connected to.
var DualOidcServers = ConfigSet{
	Name:       "dual-oidc-servers",
	CtrlConfig: "testdata/configs/dual-oidc-servers/ctrl.yml",
}

// OidcListenerBindFailure is a controller-only config set with a second web server whose bind
// point interface is an unbindable address, so its listener fails at startup. Used to verify that
// a web server whose listener cannot bind causes the controller to log the error and keep the
// other servers running, rather than panicking on a nil listener.
var OidcListenerBindFailure = ConfigSet{
	Name:       "oidc-listener-bind-failure",
	CtrlConfig: "testdata/configs/oidc-listener-bind-failure/ctrl.yml",
}

// WildcardOidcServer is a controller-only config set that supplies a wildcard (*.wildcard.test) server
// certificate via alt_server_certs alongside an ordinary primary server cert, modeling a controller fronted
// by a wildcard (e.g. LetsEncrypt) certificate. Used to verify that the OIDC discovery document served to a
// concrete host under the wildcard advertises an issuer derived from that request host, rather than a
// literal-wildcard issuer or a 404.
var WildcardOidcServer = ConfigSet{
	Name:       "wildcard-oidc-server",
	CtrlConfig: "testdata/configs/wildcard-oidc-server/ctrl.yml",
}
