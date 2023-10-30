package create

import (
	"fmt"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

/* BEGIN Controller config template structure */

type ControllerConfig struct {
	V            string       `yaml:"v"`
	Db           string       `yaml:"db"`
	Identity     Identity     `yaml:"identity"`
	Ctrl         Ctrl         `yaml:"ctrl"`
	HealthChecks HealthChecks `yaml:"healthChecks"`
	Edge         Edge         `yaml:"edge"`
	Web          []Web        `yaml:"web"`
}

type Identity struct {
	Cert       string `yaml:"cert"`
	ServerCert string `yaml:"server_cert"`
	Key        string `yaml:"key"`
	Ca         string `yaml:"ca"`
}

type Ctrl struct {
	Listener string `yaml:"listener"`
}

type HealthChecks struct {
	BoltCheck BoltCheck `yaml:"boltCheck"`
}

type BoltCheck struct {
	Interval     string `yaml:"interval"`
	Timeout      string `yaml:"timeout"`
	InitialDelay string `yaml:"initialDelay"`
}

type Edge struct {
	Api        Api        `yaml:"api"`
	Enrollment Enrollment `yaml:"enrollment"`
}

type Api struct {
	SessionTimeout string `yaml:"sessionTimeout"`
	Address        string `yaml:"address"`
}

type Enrollment struct {
	SigningCert  SigningCert  `yaml:"signingCert"`
	EdgeIdentity EdgeIdentity `yaml:"edgeIdentity"`
	EdgeRouter   EdgeRouter   `yaml:"edgeRouter"`
}

type SigningCert struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

type EdgeIdentity struct {
	Duration string `yaml:"duration"`
}

type EdgeRouter struct {
	Duration string `yaml:"duration"`
}

type Web struct {
	Name       string       `yaml:"name"`
	BindPoints []BindPoints `yaml:"bindPoints"`
	Identity   Identity     `yaml:"identity"`
	Options    Options      `yaml:"options"`
	Apis       []Apis       `yaml:"apis"`
}

type BindPoints struct {
	BpInterface string `yaml:"interface"`
	Address     string `yaml:"address"`
}

type Options struct {
	IdleTimeout   string `yaml:"idleTimeout"`
	ReadTimeout   string `yaml:"readTimeout"`
	WriteTimeout  string `yaml:"writeTimeout"`
	MinTLSVersion string `yaml:"minTLSVersion"`
	MaxTLSVersion string `yaml:"maxTLSVersion"`
}

type Apis struct {
	Binding string     `yaml:"binding"`
	Options ApiOptions `yaml:"options"`
}

type ApiOptions struct {
	// Unsure of this format right now
}

/* END Controller config template structure */
var hostname string

func init() {
	hostname, _ = os.Hostname()
}

func TestControllerOutputPathDoesNotExist(t *testing.T) {
	// Create the options with non-existent path
	options := &CreateConfigControllerOptions{}
	options.Output = "/IDoNotExist/MyController.yaml"

	err := options.run(&ConfigTemplateValues{})

	//check wrapped error type and not internal strings as they vary between operating systems
	assert.Error(t, err)
	assert.Equal(t, errors.Unwrap(err), syscall.ENOENT)
}

func TestCreateConfigControllerTemplateValues(t *testing.T) {

	// Create and run the CLI command (capture output, otherwise config prints to stdout instead of test results)
	// This must run first, otherwise the addresses used later won't be correct; this command re-allocates the `data` struct
	_, data := execCreateConfigControllerCommand(nil, nil)

	expectedNonEmptyStringFields := []string{
		".ZitiHome",
		".Controller.Identity.Cert",
		".Controller.Identity.ServerCert",
		".Controller.Identity.Key",
		".Controller.Identity.Ca",
		".Controller.Ctrl.BindAddress",
		".Controller.Ctrl.AdvertisedAddress",
		".Controller.Ctrl.AdvertisedPort",
		".Controller.EdgeApi.Address",
		".Controller.EdgeApi.Port",
		".Controller.EdgeEnrollment.SigningCert",
		".Controller.EdgeEnrollment.SigningCertKey",
		".Controller.Web.BindPoints.InterfaceAddress",
		".Controller.Web.BindPoints.InterfacePort",
		".Controller.Web.BindPoints.AddressAddress",
		".Controller.Web.BindPoints.AddressPort",
		".Controller.Web.Identity.Ca",
		".Controller.Web.Identity.Key",
		".Controller.Web.Identity.ServerCert",
		".Controller.Web.Identity.Cert",
		".Controller.Web.Options.MinTLSVersion",
		".Controller.Web.Options.MaxTLSVersion",
	}
	expectedNonEmptyStringValues := []*string{
		&data.ZitiHome,
		&data.Controller.Identity.Cert,
		&data.Controller.Identity.ServerCert,
		&data.Controller.Identity.Key,
		&data.Controller.Identity.Ca,
		&data.Controller.Ctrl.BindAddress,
		&data.Controller.Ctrl.AdvertisedAddress,
		&data.Controller.Ctrl.AdvertisedPort,
		&data.Controller.EdgeApi.Address,
		&data.Controller.EdgeApi.Port,
		&data.Controller.EdgeEnrollment.SigningCert,
		&data.Controller.EdgeEnrollment.SigningCertKey,
		&data.Controller.Web.BindPoints.InterfaceAddress,
		&data.Controller.Web.BindPoints.InterfacePort,
		&data.Controller.Web.BindPoints.AddressAddress,
		&data.Controller.Web.BindPoints.AddressPort,
		&data.Controller.Web.Identity.Ca,
		&data.Controller.Web.Identity.Key,
		&data.Controller.Web.Identity.ServerCert,
		&data.Controller.Web.Identity.Cert,
		&data.Controller.Web.Options.MinTLSVersion,
		&data.Controller.Web.Options.MaxTLSVersion,
	}

	expectedNonZeroTimeFields := []string{
		".Controller.Ctrl.MinConnectTimeout",
		".Controller.Ctrl.MaxConnectTimeout",
		".Controller.Ctrl.DefaultConnectTimeout",
		".Controller.HealthChecks.Interval",
		".Controller.HealthChecks.Timeout",
		".Controller.HealthChecks.InitialDelay",
		".Controller.EdgeApi.APIActivityUpdateInterval",
		".Controller.EdgeApi.SessionTimeout",
		".Controller.EdgeEnrollment.DefaultEdgeIdentityDuration",
		".Controller.EdgeEnrollment.EdgeIdentityDuration",
		".Controller.EdgeEnrollment.DefaultEdgeRouterDuration",
		".Controller.EdgeEnrollment.EdgeRouterDuration",
		".Controller.Web.Options.IdleTimeout",
		".Controller.Web.Options.ReadTimeout",
		".Controller.Web.Options.WriteTimeout",
	}

	expectedNonZeroTimeValues := []*time.Duration{
		&data.Controller.Ctrl.MinConnectTimeout,
		&data.Controller.Ctrl.MaxConnectTimeout,
		&data.Controller.Ctrl.DefaultConnectTimeout,
		&data.Controller.HealthChecks.Interval,
		&data.Controller.HealthChecks.Timeout,
		&data.Controller.HealthChecks.InitialDelay,
		&data.Controller.EdgeApi.APIActivityUpdateInterval,
		&data.Controller.EdgeApi.SessionTimeout,
		&data.Controller.EdgeEnrollment.DefaultEdgeIdentityDuration,
		&data.Controller.EdgeEnrollment.EdgeIdentityDuration,
		&data.Controller.EdgeEnrollment.DefaultEdgeRouterDuration,
		&data.Controller.EdgeEnrollment.EdgeRouterDuration,
		&data.Controller.Web.Options.IdleTimeout,
		&data.Controller.Web.Options.ReadTimeout,
		&data.Controller.Web.Options.WriteTimeout,
	}

	expectedNonZeroIntFields := []string{
		".Controller.Ctrl.DefaultQueuedConnects",
		".Controller.Ctrl.MinOutstandingConnects",
		".Controller.Ctrl.MaxOutstandingConnects",
		".Controller.Ctrl.DefaultOutstandingConnects",
		".Controller.EdgeApi.APIActivityUpdateBatchSize",
	}

	expectedNonZeroIntValues := []*int{
		&data.Controller.Ctrl.DefaultQueuedConnects,
		&data.Controller.Ctrl.MinOutstandingConnects,
		&data.Controller.Ctrl.MaxOutstandingConnects,
		&data.Controller.Ctrl.DefaultOutstandingConnects,
		&data.Controller.EdgeApi.APIActivityUpdateBatchSize,
	}

	// Check that the expected string template values are not blank
	for field, value := range expectedNonEmptyStringValues {
		assert.NotEqualf(t, "", *value, expectedNonEmptyStringFields[field]+" should be a non-blank value")
	}

	// Check that the expected time.Duration template values are not zero
	for field, value := range expectedNonZeroTimeValues {
		assert.NotZero(t, *value, expectedNonZeroTimeFields[field]+" should be a non-zero value")
	}

	// Check that the expected integer template values are not zero
	for field, value := range expectedNonZeroIntValues {
		assert.NotZero(t, *value, expectedNonZeroIntFields[field]+" should be a non-zero value")
	}
}

func TestCtrlConfigDefaultsWhenUnset(t *testing.T) {
	ctrlConfig, data := execCreateConfigControllerCommand(nil, nil)

	// identity:
	t.Run("TestPKICert", func(t *testing.T) {
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".cert"

		assert.Equal(t, expectedValue, data.Controller.Identity.Cert)
		assert.Equal(t, expectedValue, ctrlConfig.Identity.Cert)
	})
	t.Run("TestPKIServerCert", func(t *testing.T) {
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".server.chain.cert"

		assert.Equal(t, expectedValue, data.Controller.Identity.ServerCert)
		assert.Equal(t, expectedValue, ctrlConfig.Identity.ServerCert)
	})
	t.Run("TestPKIKey", func(t *testing.T) {
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".key"

		assert.Equal(t, expectedValue, data.Controller.Identity.Key)
		assert.Equal(t, expectedValue, ctrlConfig.Identity.Key)
	})
	t.Run("TestPKICA", func(t *testing.T) {
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".ca"

		assert.Equal(t, expectedValue, data.Controller.Identity.Ca)
		assert.Equal(t, expectedValue, ctrlConfig.Identity.Ca)
	})

	// ctrl:
	t.Run("TestBindAddress", func(t *testing.T) {
		expectedValue := testDefaultCtrlBindAddress

		assert.Equal(t, expectedValue, data.Controller.Ctrl.BindAddress)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Ctrl.Listener, ":")[1])
	})
	t.Run("TestAdvertisedPort", func(t *testing.T) {
		expectedValue := testDefaultCtrlAdvertisedPort

		assert.Equal(t, expectedValue, data.Controller.Ctrl.AdvertisedPort)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Ctrl.Listener, ":")[2])
	})

	// healthChecks:
	t.Run("TestBoltCheckInterval", func(t *testing.T) {
		expectedValue := testDefaultBoltCheckInterval

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.HealthChecks.Interval.Seconds(), "s"))
		assert.Equal(t, expectedValue, ctrlConfig.HealthChecks.BoltCheck.Interval)
	})
	t.Run("TestBoltCheckTimeout", func(t *testing.T) {
		expectedValue := testDefaultBoltCheckTimeout

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.HealthChecks.Timeout.Seconds(), "s"))
		assert.Equal(t, expectedValue, ctrlConfig.HealthChecks.BoltCheck.Timeout)
	})
	t.Run("TestBoltCheckInitialDelay", func(t *testing.T) {
		expectedValue := testDefaultBoltCheckInitialDelay

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.HealthChecks.InitialDelay.Seconds(), "s"))
		assert.Equal(t, expectedValue, ctrlConfig.HealthChecks.BoltCheck.InitialDelay)
	})

	// edge
	t.Run("TestEdgeAPIAddress", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Address
		expectedValue := data.Controller.Web.BindPoints.AddressAddress

		assert.Equal(t, expectedValue, data.Controller.EdgeApi.Address)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Edge.Api.Address, ":")[0])
	})
	t.Run("TestEdgeAPIPort", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Port
		expectedValue := data.Controller.Web.BindPoints.AddressPort

		assert.Equal(t, expectedValue, data.Controller.EdgeApi.Port)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Edge.Api.Address, ":")[1])
	})
	t.Run("TestEdgeEnrollmentSigningCert", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Port
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".signing.cert"

		assert.Equal(t, expectedValue, data.Controller.EdgeEnrollment.SigningCert)
		assert.Equal(t, expectedValue, ctrlConfig.Edge.Enrollment.SigningCert.Cert)
	})
	t.Run("TestEdgeEnrollmentSigningKey", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Port
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".signing.key"

		assert.Equal(t, expectedValue, data.Controller.EdgeEnrollment.SigningCertKey)
		assert.Equal(t, expectedValue, ctrlConfig.Edge.Enrollment.SigningCert.Key)
	})

	// web:bindPoints
	t.Run("TestEdgeBindpointInterfaceAddress", func(t *testing.T) {
		// Should default to the value of Ctrl Listener/bind Address
		expectedValue := data.Controller.Ctrl.BindAddress

		assert.Equal(t, expectedValue, data.Controller.Web.BindPoints.InterfaceAddress)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].BpInterface, ":")[0])
	})
	t.Run("TestEdgeBindpointInterfacePort", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Port
		expectedValue := data.Controller.Web.BindPoints.AddressPort

		assert.Equal(t, expectedValue, data.Controller.Web.BindPoints.InterfacePort)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].BpInterface, ":")[1])
	})
	t.Run("TestEdgeAdvertisedAddress", func(t *testing.T) {
		// Should default to hostname
		expectedValue, _ := os.Hostname()

		assert.Equal(t, expectedValue, data.Controller.Web.BindPoints.AddressAddress)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].Address, ":")[0])
	})
	t.Run("TestEdgeAdvertisedPort", func(t *testing.T) {
		// Should default to the Const value
		expectedValue := testDefaultCtrlEdgeAdvertisedPort

		assert.Equal(t, expectedValue, data.Controller.Web.BindPoints.AddressPort)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].Address, ":")[1])
	})
	t.Run("TestEdgeIdentityEnrollmentDuration", func(t *testing.T) {
		assert.Equal(t, testDefaultEdgeIdentityEnrollmentDuration, data.Controller.EdgeEnrollment.EdgeIdentityDuration)
		assert.Equal(t, testDefaultEdgeIdentityEnrollmentStr, ctrlConfig.Edge.Enrollment.EdgeIdentity.Duration)
	})
	t.Run("TestEdgeRouterEnrollmentDuration", func(t *testing.T) {
		assert.Equal(t, testDefaultEdgeRouterEnrollmentDuration, data.Controller.EdgeEnrollment.EdgeRouterDuration)
		assert.Equal(t, testDefaultEdgeRouterEnrollmentStr, ctrlConfig.Edge.Enrollment.EdgeRouter.Duration)
	})

	// web:identity
	t.Run("TestEdgePKICert", func(t *testing.T) {
		// Defaults to the Controller identity ca
		expectedValue := data.Controller.Identity.Ca

		assert.Equal(t, expectedValue, data.Controller.Web.Identity.Ca)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Identity.Ca)
	})
	t.Run("TestEdgePKIServerCert", func(t *testing.T) {
		// Defaults to the Controller identity key
		expectedValue := data.Controller.Identity.Key

		assert.Equal(t, expectedValue, data.Controller.Web.Identity.Key)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Identity.Key)
	})
	t.Run("TestEdgePKIKey", func(t *testing.T) {
		// Defaults to the Controller identity server cert
		expectedValue := data.Controller.Identity.ServerCert

		assert.Equal(t, expectedValue, data.Controller.Web.Identity.ServerCert)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Identity.ServerCert)
	})
	t.Run("TestEdgePKICA", func(t *testing.T) {
		// Defaults to the Controller identity cert
		expectedValue := data.Controller.Identity.Cert

		assert.Equal(t, expectedValue, data.Controller.Web.Identity.Cert)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Identity.Cert)
	})

	// web:options
	t.Run("TestEdgeOptionsIdleTimeout", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsIdleTimeout

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.Web.Options.IdleTimeout.Milliseconds(), "ms"))
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.IdleTimeout)
	})
	t.Run("TestEdgeOptionsReadTimeout", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsReadTimeout

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.Web.Options.ReadTimeout.Milliseconds(), "ms"))
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.ReadTimeout)
	})
	t.Run("TestEdgeOptionsWriteTimeout", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsWriteTimeout

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.Web.Options.WriteTimeout.Milliseconds(), "ms"))
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.WriteTimeout)
	})
	t.Run("TestEdgeOptionsMinTLS", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsMinTLSVersion

		assert.Equal(t, expectedValue, data.Controller.Web.Options.MinTLSVersion)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.MinTLSVersion)
	})
	t.Run("TestEdgeOptionsMaxTLS", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsMaxTLSVersion

		assert.Equal(t, expectedValue, data.Controller.Web.Options.MaxTLSVersion)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.MaxTLSVersion)
	})
}

func TestCtrlConfigDefaultsWhenBlank(t *testing.T) {
	keys := map[string]string{
		"ZITI_PKI_CTRL_CERT":                "",
		"ZITI_CTRL_EDGE_ADVERTISED_ADDRESS": "",
		"ZITI_HOME":                         "",
	}
	// run the config
	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	// identity:
	t.Run("TestPKICert", func(t *testing.T) {
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".cert"

		assert.Equal(t, expectedValue, data.Controller.Identity.Cert)
		assert.Equal(t, expectedValue, ctrlConfig.Identity.Cert)
	})
	t.Run("TestPKIServerCert", func(t *testing.T) {
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".server.chain.cert"

		assert.Equal(t, expectedValue, data.Controller.Identity.ServerCert)
		assert.Equal(t, expectedValue, ctrlConfig.Identity.ServerCert)
	})
	t.Run("TestPKIKey", func(t *testing.T) {
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".key"

		assert.Equal(t, expectedValue, data.Controller.Identity.Key)
		assert.Equal(t, expectedValue, ctrlConfig.Identity.Key)
	})
	t.Run("TestPKICA", func(t *testing.T) {
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".ca"

		assert.Equal(t, expectedValue, data.Controller.Identity.Ca)
		assert.Equal(t, expectedValue, ctrlConfig.Identity.Ca)
	})

	// ctrl:
	t.Run("TestBindAddress", func(t *testing.T) {
		expectedValue := testDefaultCtrlBindAddress

		assert.Equal(t, expectedValue, data.Controller.Ctrl.BindAddress)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Ctrl.Listener, ":")[1])
	})
	t.Run("TestAdvertisedPort", func(t *testing.T) {
		expectedValue := testDefaultCtrlAdvertisedPort

		assert.Equal(t, expectedValue, data.Controller.Ctrl.AdvertisedPort)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Ctrl.Listener, ":")[2])
	})

	// healthChecks:
	t.Run("TestBoltCheckInterval", func(t *testing.T) {
		expectedValue := testDefaultBoltCheckInterval

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.HealthChecks.Interval.Seconds(), "s"))
		assert.Equal(t, expectedValue, ctrlConfig.HealthChecks.BoltCheck.Interval)
	})
	t.Run("TestBoltCheckTimeout", func(t *testing.T) {
		expectedValue := testDefaultBoltCheckTimeout

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.HealthChecks.Timeout.Seconds(), "s"))
		assert.Equal(t, expectedValue, ctrlConfig.HealthChecks.BoltCheck.Timeout)
	})
	t.Run("TestBoltCheckInitialDelay", func(t *testing.T) {
		expectedValue := testDefaultBoltCheckInitialDelay

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.HealthChecks.InitialDelay.Seconds(), "s"))
		assert.Equal(t, expectedValue, ctrlConfig.HealthChecks.BoltCheck.InitialDelay)
	})

	// edge
	t.Run("TestEdgeAPIAddress", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Address
		expectedValue := data.Controller.Web.BindPoints.AddressAddress

		assert.Equal(t, expectedValue, data.Controller.EdgeApi.Address)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Edge.Api.Address, ":")[0])
	})
	t.Run("TestEdgeAPIPort", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Port
		expectedValue := data.Controller.Web.BindPoints.AddressPort

		assert.Equal(t, expectedValue, data.Controller.EdgeApi.Port)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Edge.Api.Address, ":")[1])
	})
	t.Run("TestEdgeEnrollmentSigningCert", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Port
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".signing.cert"

		assert.Equal(t, expectedValue, data.Controller.EdgeEnrollment.SigningCert)
		assert.Equal(t, expectedValue, ctrlConfig.Edge.Enrollment.SigningCert.Cert)
	})
	t.Run("TestEdgeEnrollmentSigningKey", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Port
		expectedValue := cmdhelper.GetZitiHome() + "/" + cmdhelper.HostnameOrNetworkName() + ".signing.key"

		assert.Equal(t, expectedValue, data.Controller.EdgeEnrollment.SigningCertKey)
		assert.Equal(t, expectedValue, ctrlConfig.Edge.Enrollment.SigningCert.Key)
	})
	t.Run("TestEdgeIdentityEnrollmentDuration", func(t *testing.T) {
		assert.Equal(t, testDefaultEdgeIdentityEnrollmentDuration, data.Controller.EdgeEnrollment.EdgeIdentityDuration)
		assert.Equal(t, testDefaultEdgeIdentityEnrollmentStr, ctrlConfig.Edge.Enrollment.EdgeIdentity.Duration)
	})
	t.Run("TestEdgeRouterEnrollmentDuration", func(t *testing.T) {
		assert.Equal(t, testDefaultEdgeRouterEnrollmentDuration, data.Controller.EdgeEnrollment.EdgeRouterDuration)
		assert.Equal(t, testDefaultEdgeRouterEnrollmentStr, ctrlConfig.Edge.Enrollment.EdgeRouter.Duration)
	})

	// web:bindPoints
	t.Run("TestEdgeBindpointInterfaceAddress", func(t *testing.T) {
		// Should default to the value of Ctrl Listener/bind Address
		expectedValue := data.Controller.Ctrl.BindAddress

		assert.Equal(t, expectedValue, data.Controller.Web.BindPoints.InterfaceAddress)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].BpInterface, ":")[0])
	})
	t.Run("TestEdgeBindpointInterfacePort", func(t *testing.T) {
		// Should default to the value of Ctrl Edge Advertised Port
		expectedValue := data.Controller.Web.BindPoints.AddressPort

		assert.Equal(t, expectedValue, data.Controller.Web.BindPoints.InterfacePort)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].BpInterface, ":")[1])
	})
	t.Run("TestEdgeAdvertisedAddress", func(t *testing.T) {
		// Should default to hostname
		expectedValue, _ := os.Hostname()

		assert.Equal(t, expectedValue, data.Controller.Web.BindPoints.AddressAddress)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].Address, ":")[0])
	})
	t.Run("TestEdgeAdvertisedPort", func(t *testing.T) {
		// Should default to the Const value
		expectedValue := testDefaultCtrlEdgeAdvertisedPort

		assert.Equal(t, expectedValue, data.Controller.Web.BindPoints.AddressPort)
		assert.Equal(t, expectedValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].Address, ":")[1])
	})

	// web:identity
	t.Run("TestEdgePKICert", func(t *testing.T) {
		// Defaults to the Controller identity ca
		expectedValue := data.Controller.Identity.Ca

		assert.Equal(t, expectedValue, data.Controller.Web.Identity.Ca)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Identity.Ca)
	})
	t.Run("TestEdgePKIServerCert", func(t *testing.T) {
		// Defaults to the Controller identity key
		expectedValue := data.Controller.Identity.Key

		assert.Equal(t, expectedValue, data.Controller.Web.Identity.Key)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Identity.Key)
	})
	t.Run("TestEdgePKIKey", func(t *testing.T) {
		// Defaults to the Controller identity server cert
		expectedValue := data.Controller.Identity.ServerCert

		assert.Equal(t, expectedValue, data.Controller.Web.Identity.ServerCert)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Identity.ServerCert)
	})
	t.Run("TestEdgePKICA", func(t *testing.T) {
		// Defaults to the Controller identity cert
		expectedValue := data.Controller.Identity.Cert

		assert.Equal(t, expectedValue, data.Controller.Web.Identity.Cert)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Identity.Cert)
	})

	// web:options
	t.Run("TestEdgeOptionsIdleTimeout", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsIdleTimeout

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.Web.Options.IdleTimeout.Milliseconds(), "ms"))
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.IdleTimeout)
	})
	t.Run("TestEdgeOptionsReadTimeout", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsReadTimeout

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.Web.Options.ReadTimeout.Milliseconds(), "ms"))
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.ReadTimeout)
	})
	t.Run("TestEdgeOptionsWriteTimeout", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsWriteTimeout

		assert.Equal(t, expectedValue, fmt.Sprint(data.Controller.Web.Options.WriteTimeout.Milliseconds(), "ms"))
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.WriteTimeout)
	})
	t.Run("TestEdgeOptionsMinTLS", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsMinTLSVersion

		assert.Equal(t, expectedValue, data.Controller.Web.Options.MinTLSVersion)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.MinTLSVersion)
	})
	t.Run("TestEdgeOptionsMaxTLS", func(t *testing.T) {
		expectedValue := testDefaultEdgeOptionsMaxTLSVersion

		assert.Equal(t, expectedValue, data.Controller.Web.Options.MaxTLSVersion)
		assert.Equal(t, expectedValue, ctrlConfig.Web[0].Options.MaxTLSVersion)
	})
}

func TestZitiCtrlIdentitySection(t *testing.T) {
	certPath := "/var/test/custom/path/file.cert"
	serverCertPath := "/var/test/custom/path/file.chain.pem"
	keyPath := "/var/test/custom/path/file.key"
	caPath := "/var/test/custom/path/file.pem"
	keys := map[string]string{
		"ZITI_PKI_CTRL_CERT":        certPath,
		"ZITI_PKI_CTRL_SERVER_CERT": serverCertPath,
		"ZITI_PKI_CTRL_KEY":         keyPath,
		"ZITI_PKI_CTRL_CA":          caPath,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, certPath, data.Controller.Identity.Cert)
	assert.Equal(t, certPath, ctrlConfig.Identity.Cert)
	assert.Equal(t, serverCertPath, data.Controller.Identity.ServerCert)
	assert.Equal(t, serverCertPath, ctrlConfig.Identity.ServerCert)
	assert.Equal(t, keyPath, data.Controller.Identity.Key)
	assert.Equal(t, keyPath, ctrlConfig.Identity.Key)
	assert.Equal(t, caPath, data.Controller.Identity.Ca)
	assert.Equal(t, caPath, ctrlConfig.Identity.Ca)
}

func TestDefaultPkiPath(t *testing.T) {
	expectedPkiRoot := "/tmp/expectedPkiRoot"
	keys := map[string]string{
		"ZITI_HOME": expectedPkiRoot,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Contains(t, data.Controller.Identity.Cert, expectedPkiRoot)
	assert.Contains(t, data.Controller.Identity.ServerCert, expectedPkiRoot)
	assert.Contains(t, ctrlConfig.Identity.ServerCert, expectedPkiRoot)
	assert.Contains(t, data.Controller.Identity.Key, expectedPkiRoot)
	assert.Contains(t, ctrlConfig.Identity.Key, expectedPkiRoot)
	assert.Contains(t, data.Controller.Identity.Ca, expectedPkiRoot)
	assert.Contains(t, ctrlConfig.Identity.Ca, expectedPkiRoot)
}

func TestCtrlBindAddress(t *testing.T) {
	customValue := "123.456.7.8"
	keys := map[string]string{
		"ZITI_CTRL_BIND_ADDRESS": customValue,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, customValue, data.Controller.Ctrl.BindAddress)
	assert.Equal(t, customValue, strings.Split(ctrlConfig.Ctrl.Listener, ":")[1])
}

func TestCtrlAdvertisedPort(t *testing.T) {
	customValue := "9996"
	keys := map[string]string{
		"ZITI_CTRL_ADVERTISED_PORT": customValue,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, customValue, data.Controller.Ctrl.AdvertisedPort)
	assert.Equal(t, customValue, strings.Split(ctrlConfig.Ctrl.Listener, ":")[2])
}

func TestCtrlEdgeAPIAddress(t *testing.T) {
	customValue := "123.456.7.8"
	keys := map[string]string{
		"ZITI_CTRL_EDGE_ADVERTISED_ADDRESS": customValue,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, customValue, data.Controller.EdgeApi.Address)
	assert.Equal(t, customValue, strings.Split(ctrlConfig.Edge.Api.Address, ":")[0])
}

func TestCtrlEdgeAPIPort(t *testing.T) {
	customValue := "9995"
	keys := map[string]string{
		"ZITI_CTRL_EDGE_ADVERTISED_PORT": customValue,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, customValue, data.Controller.EdgeApi.Port)
	assert.Equal(t, customValue, strings.Split(ctrlConfig.Edge.Api.Address, ":")[1])
}

func TestCtrlEdgeAPIEnrollmentSignerCert(t *testing.T) {
	certPath := "/var/test/custom/path/file.cert"
	keyPath := "/var/test/custom/path/file.key"
	keys := map[string]string{
		"ZITI_PKI_SIGNER_CERT": certPath,
		"ZITI_PKI_SIGNER_KEY":  keyPath,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, certPath, data.Controller.EdgeEnrollment.SigningCert)
	assert.Equal(t, certPath, ctrlConfig.Edge.Enrollment.SigningCert.Cert)
	assert.Equal(t, keyPath, data.Controller.EdgeEnrollment.SigningCertKey)
	assert.Equal(t, keyPath, ctrlConfig.Edge.Enrollment.SigningCert.Key)
}

func TestEdgeIdentityEnrollmentDurationEnvVar(t *testing.T) {
	customDuration := time.Duration(5) * time.Minute
	customValue := "5"
	expectedValue := customValue + "m" // Env Var int is converted to minutes format
	keys := map[string]string{
		"ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION": customValue,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, customDuration, data.Controller.EdgeEnrollment.EdgeIdentityDuration)
	assert.Equal(t, expectedValue, ctrlConfig.Edge.Enrollment.EdgeIdentity.Duration)
}

func TestEdgeIdentityEnrollmentDurationCLITakesPriority(t *testing.T) {
	envVarValue := "5" // Setting a custom duration which is not the default value
	cliValue := "10m"  // Setting a CLI custom duration which is also not the default value
	expectedConfigValue := cliValue
	keys := map[string]string{
		"ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION": envVarValue,
	}
	args := []string{"--identityEnrollmentDuration", cliValue}

	ctrlConfig, data := execCreateConfigControllerCommand(args, keys)

	// Expect that the CLI value was used over the environment variable
	expectedValue, _ := time.ParseDuration(cliValue)
	assert.Equal(t, expectedValue, data.Controller.EdgeEnrollment.EdgeIdentityDuration)

	// Expect that the config value is represented correctly
	assert.Equal(t, expectedConfigValue, ctrlConfig.Edge.Enrollment.EdgeIdentity.Duration)
}

func TestEdgeIdentityEnrollmentDurationCLIConvertsToMin(t *testing.T) {
	cliValue := "1h"             // Setting a CLI custom duration which is also not the default value
	expectedConfigValue := "60m" // Config value representation should be in minutes

	args := []string{"--identityEnrollmentDuration", cliValue}
	ctrlConfig, data := execCreateConfigControllerCommand(args, nil)

	// Expect that the CLI value was used over the environment variable
	expectedValue, _ := time.ParseDuration(cliValue)
	assert.Equal(t, expectedValue, data.Controller.EdgeEnrollment.EdgeIdentityDuration)

	// Expect that the config value is represented correctly
	assert.Equal(t, expectedConfigValue, ctrlConfig.Edge.Enrollment.EdgeIdentity.Duration)
}

func TestEdgeRouterEnrollmentDurationEnvVar(t *testing.T) {
	customDuration := time.Duration(5) * time.Minute
	customValue := "5"
	expectedValue := customValue + "m" // Env Var int is converted to minutes format
	keys := map[string]string{
		"ZITI_ROUTER_ENROLLMENT_DURATION": customValue,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, customDuration, data.Controller.EdgeEnrollment.EdgeRouterDuration)
	assert.Equal(t, expectedValue, ctrlConfig.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterEnrollmentDurationCLITakesPriority(t *testing.T) {
	envVarValue := "5" // Setting a custom duration which is not the default value
	cliValue := "10m"  // Setting a CLI custom duration which is also not the default value
	expectedConfigValue := cliValue
	keys := map[string]string{
		"ZITI_ROUTER_ENROLLMENT_DURATION": envVarValue,
	}
	args := []string{"--routerEnrollmentDuration", cliValue}

	ctrlConfig, data := execCreateConfigControllerCommand(args, keys)

	// Expect that the CLI value was used over the environment variable
	expectedValue, _ := time.ParseDuration(cliValue)
	assert.Equal(t, expectedValue, data.Controller.EdgeEnrollment.EdgeRouterDuration)

	// Expect that the config value is represented correctly
	assert.Equal(t, expectedConfigValue, ctrlConfig.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterEnrollmentDurationCLIConvertsToMin(t *testing.T) {
	cliValue := "1h"             // Setting a CLI custom duration which is also not the default value
	expectedConfigValue := "60m" // Config value representation should be in minutes

	args := []string{"--routerEnrollmentDuration", cliValue}
	ctrlConfig, data := execCreateConfigControllerCommand(args, nil)

	// Expect that the CLI value was used over the environment variable
	expectedValue, _ := time.ParseDuration(cliValue)
	assert.Equal(t, expectedValue, data.Controller.EdgeEnrollment.EdgeRouterDuration)

	// Expect that the config value is represented correctly
	assert.Equal(t, expectedConfigValue, ctrlConfig.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterAndIdentityEnrollmentDurationTogetherCLI(t *testing.T) {
	cliIdentityDurationValue := "1h"
	cliRouterDurationValue := "30m"
	expectedIdentityConfigValue := "60m"
	expectedRouterConfigValue := "30m"

	// Create and run the CLI command
	args := []string{"--routerEnrollmentDuration", cliRouterDurationValue, "--identityEnrollmentDuration", cliIdentityDurationValue}
	configStruct, _ := execCreateConfigControllerCommand(args, nil)

	// Expect that the config values are represented correctly
	assert.Equal(t, expectedIdentityConfigValue, configStruct.Edge.Enrollment.EdgeIdentity.Duration)
	assert.Equal(t, expectedRouterConfigValue, configStruct.Edge.Enrollment.EdgeRouter.Duration)
}

func TestEdgeRouterAndIdentityEnrollmentDurationTogetherEnvVar(t *testing.T) {
	envVarIdentityDurationValue := "120"
	envVarRouterDurationValue := "60"
	expectedIdentityConfigValue := envVarIdentityDurationValue + "m"
	expectedRouterConfigValue := envVarRouterDurationValue + "m"

	// Create and run the CLI command
	keys := map[string]string{
		"ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION": envVarIdentityDurationValue,
		"ZITI_ROUTER_ENROLLMENT_DURATION":        envVarRouterDurationValue,
	}
	configStruct, _ := execCreateConfigControllerCommand(nil, keys)

	// Expect that the config values are represented correctly
	assert.Equal(t, expectedIdentityConfigValue, configStruct.Edge.Enrollment.EdgeIdentity.Duration)
	assert.Equal(t, expectedRouterConfigValue, configStruct.Edge.Enrollment.EdgeRouter.Duration)
}

func TestCtrlEdgeInterfaceAddress(t *testing.T) {
	addy := "custom.domain.name"
	port := "9998"
	keys := map[string]string{
		"ZITI_CTRL_EDGE_BIND_ADDRESS":    addy,
		"ZITI_CTRL_EDGE_ADVERTISED_PORT": port,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, addy, data.Controller.Web.BindPoints.InterfaceAddress)
	assert.Equal(t, addy, strings.Split(ctrlConfig.Web[0].BindPoints[0].BpInterface, ":")[0])
	assert.Equal(t, port, strings.Split(ctrlConfig.Web[0].BindPoints[0].BpInterface, ":")[1])
}

func TestCtrlEdgeAdvertisedAddress(t *testing.T) {
	customValue := "123.456.7.8"
	keys := map[string]string{
		"ZITI_CTRL_EDGE_ADVERTISED_ADDRESS": customValue,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, customValue, data.Controller.Web.BindPoints.AddressAddress)
	assert.Equal(t, customValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].Address, ":")[0])
}

func TestCtrlEdgeAdvertisedPort(t *testing.T) {
	customValue := "9997"
	keys := map[string]string{
		"ZITI_CTRL_EDGE_ADVERTISED_PORT": customValue,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, customValue, data.Controller.Web.BindPoints.AddressPort)
	assert.Equal(t, customValue, strings.Split(ctrlConfig.Web[0].BindPoints[0].Address, ":")[1])
}

func TestCtrlEdgeIdentitySection(t *testing.T) {
	caPath := "/var/test/custom/path/file.pem"
	keyPath := "/var/test/custom/path/file.key"
	serverCertPath := "/var/test/custom/path/file.chain.pem"
	certPath := "/var/test/custom/path/file.cert"
	keys := map[string]string{
		"ZITI_PKI_CTRL_CA":          caPath,
		"ZITI_PKI_CTRL_KEY":         keyPath,
		"ZITI_PKI_CTRL_SERVER_CERT": serverCertPath,
		"ZITI_PKI_CTRL_CERT":        certPath,
	}

	ctrlConfig, data := execCreateConfigControllerCommand(nil, keys)

	assert.Equal(t, certPath, data.Controller.Web.Identity.Cert)
	assert.Equal(t, certPath, ctrlConfig.Web[0].Identity.Cert)
	assert.Equal(t, serverCertPath, data.Controller.Web.Identity.ServerCert)
	assert.Equal(t, serverCertPath, ctrlConfig.Web[0].Identity.ServerCert)
	assert.Equal(t, keyPath, data.Controller.Web.Identity.Key)
	assert.Equal(t, keyPath, ctrlConfig.Web[0].Identity.Key)
	assert.Equal(t, caPath, data.Controller.Web.Identity.Ca)
	assert.Equal(t, caPath, ctrlConfig.Web[0].Identity.Ca)
}

func TestCtrlEdgeAltAddress(t *testing.T) {
	// first test when it's not set
	ctrlConfig, data := execCreateConfigControllerCommand(nil, map[string]string{})
	assert.Equal(t, hostname, data.Controller.Ctrl.AltAdvertisedAddress)
	assert.Equal(t, hostname+":"+testDefaultCtrlEdgeAdvertisedPort, ctrlConfig.Web[0].BindPoints[0].Address)
	assert.Equal(t, hostname+":"+testDefaultCtrlEdgeAdvertisedPort, ctrlConfig.Edge.Api.Address)

	altAddy := "alternative.address.ziti"
	keys := map[string]string{
		"ZITI_CTRL_EDGE_ALT_ADVERTISED_ADDRESS": altAddy,
	}
	ctrlConfig2, data2 := execCreateConfigControllerCommand(nil, keys)
	assert.Equal(t, altAddy, data2.Controller.Ctrl.AltAdvertisedAddress)
	assert.Equal(t, altAddy+":"+testDefaultCtrlEdgeAdvertisedPort, ctrlConfig2.Web[0].BindPoints[0].Address)
	assert.Equal(t, altAddy+":"+testDefaultCtrlEdgeAdvertisedPort, ctrlConfig2.Edge.Api.Address)
}

func configToStruct(config string) ControllerConfig {
	configStruct := ControllerConfig{}
	err2 := yaml.Unmarshal([]byte(config), &configStruct)
	if err2 != nil {
		fmt.Println(err2)
	}
	return configStruct
}

func execCreateConfigControllerCommand(args []string, keys map[string]string) (ControllerConfig, *ConfigTemplateValues) {
	// Setup
	clearEnvAndInitializeTestData()
	controllerOptions := CreateConfigControllerOptions{}
	controllerOptions.Output = defaultOutput

	setEnvByMap(keys)
	// Create and run the CLI command (capture output to convert to a template struct)
	cmd := NewCmdCreateConfigController()
	cmd.SetArgs(args)
	configOutput := captureOutput(func() {
		_ = cmd.Execute()
	})

	return configToStruct(configOutput), cmd.ConfigData
}
