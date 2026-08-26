/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package config

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/pkg/errors"
	"golang.org/x/net/idna"
)

const (
	DefaultEdgeApiActivityUpdateBatchSize = 250
	DefaultEdgeAPIActivityUpdateInterval  = 90 * time.Second
	MaxEdgeAPIActivityUpdateBatchSize     = 10000
	MinEdgeAPIActivityUpdateBatchSize     = 1
	MaxEdgeAPIActivityUpdateInterval      = 10 * time.Minute
	MinEdgeAPIActivityUpdateInterval      = time.Millisecond

	DefaultEdgeSessionTimeout = 30 * time.Minute
	MinEdgeSessionTimeout     = 1 * time.Minute

	MinEdgeEnrollmentDuration     = 5 * time.Minute
	DefaultEdgeEnrollmentDuration = 180 * time.Minute

	DefaultHttpIdleTimeout       = 5000 * time.Millisecond
	DefaultHttpReadTimeout       = 5000 * time.Millisecond
	DefaultHttpReadHeaderTimeout = 5000 * time.Millisecond
	DefaultHttpWriteTimeout      = 100000 * time.Millisecond

	DefaultTotpDomain = "openziti.io"

	DefaultAuthRateLimiterEnabled = true
	DefaultAuthRateLimiterMaxSize = 250
	DefaultAuthRateLimiterMinSize = 5

	AuthRateLimiterMinSizeValue = 5
	AuthRateLimiterMaxSizeValue = 1000

	DefaultIdentityOnlineStatusScanInterval = time.Minute
	MinIdentityOnlineStatusScanInterval     = time.Second

	DefaultIdentityOnlineStatusUnknownTimeout = 5 * time.Minute
	DefaultIdentityOnlineStatusSource         = IdentityStatusSourceHybrid

	// DefaultJwksFetchBlockPrivateAddresses leaves private and loopback addresses reachable by
	// default so that deployments using an internal IdP keep working. Metadata and link-local
	// addresses are blocked regardless of this setting.
	DefaultJwksFetchBlockPrivateAddresses = false

	// DefaultJwksFetchTimeout bounds the total time spent fetching a JWKS endpoint.
	DefaultJwksFetchTimeout = 5 * time.Second

	// DefaultJwksFetchMaxRedirects bounds how many redirects a JWKS fetch will follow.
	DefaultJwksFetchMaxRedirects = 5
)

type Enrollment struct {
	SigningCert       identity.Identity
	SigningCertConfig identity.Config
	SigningCertCaPem  []byte
	EdgeIdentity      EnrollmentOption
	EdgeRouter        EnrollmentOption
}

type EnrollmentOption struct {
	Duration time.Duration
}

type Totp struct {
	Hostname string
}

type Api struct {
	SessionTimeout          time.Duration
	ActivityUpdateBatchSize int
	ActivityUpdateInterval  time.Duration

	Listener               string
	Address                string
	IdentityCaPem          []byte
	HttpTimeouts           HttpTimeouts
	DisableOidcAutoBinding bool
}

type Oidc struct {
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	IdTokenDuration      time.Duration

	// RevocationMinTokenLifetime skips revocation for refresh tokens that expire
	// within this duration. Unset (zero) means always revoke. Must be less than
	// 50% of RefreshTokenDuration if set.
	RevocationMinTokenLifetime time.Duration

	// RevocationBucketInterval is the bucket window for batching refresh-token
	// revocations before flushing them through raft.
	RevocationBucketInterval time.Duration

	// RevocationBucketMaxSize is the maximum number of revocations per raft
	// log entry / DB transaction when flushing.
	RevocationBucketMaxSize int

	// RevocationMaxQueued is the maximum number of revocations that can
	// accumulate in memory before new ones are dropped.
	RevocationMaxQueued int

	// RevocationEnforcerFrequency is how often the controller purges expired
	// revocation records from the database.
	RevocationEnforcerFrequency time.Duration
}

// MaxTokenDuration returns the longest of the configured refresh, access, and id
// token durations. Revocations are set to expire after this so they outlive any
// token they target.
func (o *Oidc) MaxTokenDuration() time.Duration {
	return common.MaxTokenDuration(o.RefreshTokenDuration, o.AccessTokenDuration, o.IdTokenDuration)
}

// ExternalJwtSigners holds settings that govern how the controller interacts with
// external JWT signers.
type ExternalJwtSigners struct {
	JwksFetch JwksFetch
}

// JwksFetch controls the server-side fetch of an external JWT signer's jwksEndpoint.
// The endpoint URL is supplied by an operator, so the fetch is constrained to keep it
// from being pointed at addresses the controller can reach but a caller should not.
//
// A hop is fetched only if it passes two independent gates. Neither gate can authorize what
// the other refuses, and both are applied to the initial request and to every redirect.
//
// The host gate is applied to the URL's hostname:
//
//  1. DeniedHostnames - blocked
//  2. AllowedHostnames, when non-empty and the host does not match - blocked
//  3. otherwise - passes
//
// The address gate is applied to the resolved address being connected to, first-match-wins,
// deny before allow:
//
//  1. built-in blocked addresses (cloud metadata, link-local, link-local multicast,
//     unspecified) - always blocked, AllowedIPs cannot override
//  2. DeniedIPs - blocked, AllowedIPs cannot override
//  3. AllowedIPs - allowed; a carve-out of tier 4 only
//  4. BlockPrivateAddresses and the address is private or loopback - blocked
//  5. everything else - allowed
type JwksFetch struct {
	// BlockPrivateAddresses blocks private and loopback addresses (address gate tier 4).
	// Defaults to false so deployments with an internal IdP keep working; tier 1 applies
	// regardless.
	BlockPrivateAddresses bool

	// DeniedIPs are CIDRs that are always blocked (address gate tier 2), above
	// AllowedIPs.
	DeniedIPs []*net.IPNet

	// AllowedIPs are CIDRs that carve an exception out of BlockPrivateAddresses
	// (address gate tier 3). They do not override tier 1 or DeniedIPs.
	AllowedIPs []*net.IPNet

	// DeniedHostnames are normalized hostname patterns that are blocked (hostname gate tier 1). Host
	// matching only ever narrows what may be fetched: it cannot authorize an address the
	// address gate blocks, and a caller can still reach the same target under another name,
	// so the address gate remains the boundary.
	DeniedHostnames []string

	// AllowedHostnames are normalized hostname patterns that, when non-empty, are the only hosts that
	// may be fetched (hostname gate tier 2). Entries are an exact hostname (idp.example.com) or a
	// wildcard suffix (*.example.com), which matches any subdomain but not the suffix itself.
	AllowedHostnames []string

	// Timeout bounds the total time spent on a single JWKS fetch, including redirects.
	Timeout time.Duration

	// MaxRedirects bounds how many redirects a JWKS fetch will follow. Every hop is
	// address-checked. Zero disables redirects.
	MaxRedirects int
}

type EdgeConfig struct {
	Enabled              bool
	Api                  Api
	Oidc                 Oidc
	Enrollment           Enrollment
	IdentityStatusConfig IdentityStatusConfig
	caPems               *bytes.Buffer
	caPemsOnce           sync.Once
	Totp                 Totp
	AuthRateLimiter      command.AdaptiveRateLimiterConfig
	caCerts              []*x509.Certificate
	caCertPool           *x509.CertPool
	DisablePostureChecks bool
	ExternalJwtSigners   ExternalJwtSigners
}

type HttpTimeouts struct {
	ReadTimeoutDuration       time.Duration
	ReadHeaderTimeoutDuration time.Duration
	WriteTimeoutDuration      time.Duration
	IdleTimeoutsDuration      time.Duration
}

func DefaultHttpTimeouts() *HttpTimeouts {
	httpTimeouts := &HttpTimeouts{
		ReadTimeoutDuration:       DefaultHttpReadTimeout,
		ReadHeaderTimeoutDuration: DefaultHttpReadHeaderTimeout,
		WriteTimeoutDuration:      DefaultHttpWriteTimeout,
		IdleTimeoutsDuration:      DefaultHttpIdleTimeout,
	}
	return httpTimeouts
}

type IdentityStatusSource uint32

const (
	IdentityStatusSourceHeartbeats    IdentityStatusSource = 1
	IdentityStatusSourceConnectEvents IdentityStatusSource = 2
	IdentityStatusSourceHybrid        IdentityStatusSource = 3
)

type IdentityStatusConfig struct {
	Source         IdentityStatusSource
	ScanInterval   time.Duration
	UnknownTimeout time.Duration
}

// DefaultJwksFetch returns the default JWKS fetch settings. The defaults are deliberately
// compatible with existing deployments: only the non-disableable built-in blocked addresses
// are refused.
func DefaultJwksFetch() JwksFetch {
	return JwksFetch{
		BlockPrivateAddresses: DefaultJwksFetchBlockPrivateAddresses,
		Timeout:               DefaultJwksFetchTimeout,
		MaxRedirects:          DefaultJwksFetchMaxRedirects,
	}
}

func NewEdgeConfig() *EdgeConfig {
	return &EdgeConfig{
		Enabled: false,
		caPems:  bytes.NewBuffer(nil),
		ExternalJwtSigners: ExternalJwtSigners{
			JwksFetch: DefaultJwksFetch(),
		},
	}
}

func (c *EdgeConfig) SessionTimeoutDuration() time.Duration {
	return c.Api.SessionTimeout
}

func (c *EdgeConfig) CaPems() []byte {
	c.caPemsOnce.Do(func() {
		c.RefreshCas()
	})

	return c.caPems.Bytes()
}

func (c *EdgeConfig) CaCerts() []*x509.Certificate {
	c.caPemsOnce.Do(func() {
		c.RefreshCas()
	})

	return c.caCerts
}

func (c *EdgeConfig) CaCertsPool() *x509.CertPool {
	c.caPemsOnce.Do(func() {
		c.RefreshCas()
	})

	return c.caCertPool
}

// AddCaPems adds a byte array of certificates to the current buffered list of CAs. The certificates
// should be in PEM format separated by new lines. RefreshCas should be called after all
// calls to AddCaPems are completed.
func (c *EdgeConfig) AddCaPems(caPems []byte) {
	c.caPems.WriteString("\n")
	c.caPems.Write(caPems)
}

func (c *EdgeConfig) RefreshCas() {
	c.caPems = CalculateCaPems(c.caPems)
	c.caCerts = nfpem.PemBytesToCertificates(c.caPems.Bytes())
	c.caCertPool = x509.NewCertPool()

	for _, cert := range c.caCerts {
		c.caCertPool.AddCert(cert)
	}
}

func (c *EdgeConfig) loadTotpSection(edgeConfigMap map[any]any) error {
	c.Totp = Totp{}
	c.Totp.Hostname = DefaultTotpDomain

	if value, found := edgeConfigMap["totp"]; found {
		if value == nil {
			return nil
		}

		totpMap := value.(map[interface{}]interface{})

		if totpMap != nil {
			if hostnameVal, found := totpMap["hostname"]; found {

				if hostnameVal == nil {
					return nil
				}

				if hostname, ok := hostnameVal.(string); ok {
					testUrl := "https://" + hostname
					parsedUrl, err := url.Parse(testUrl)

					if err != nil {
						return fmt.Errorf("could not parse URL: %w", err)
					}

					if parsedUrl.Hostname() != hostname {
						return fmt.Errorf("invalid hostname in [edge.totp.hostname]: %s", hostname)
					}

					c.Totp.Hostname = hostname
				} else {
					return fmt.Errorf("[edge.totp.hostname] must be a string")
				}
			}
		}
	}

	return nil
}

func (c *EdgeConfig) loadOidcSection(edgeConfigMap map[any]any) error {
	c.Oidc.AccessTokenDuration = 30 * time.Minute
	c.Oidc.RefreshTokenDuration = 24 * time.Hour
	c.Oidc.IdTokenDuration = 30 * time.Minute
	// RevocationMinTokenLifetime defaults to 0 (unset), meaning always revoke.
	c.Oidc.RevocationBucketInterval = 1 * time.Minute
	c.Oidc.RevocationBucketMaxSize = 200
	c.Oidc.RevocationMaxQueued = 25000
	c.Oidc.RevocationEnforcerFrequency = 1 * time.Minute

	if value, found := edgeConfigMap["oidc"]; found {
		oidcSubMap := value.(map[interface{}]interface{})

		if oidcSubMap != nil {
			if val, ok := oidcSubMap["accessTokenDuration"]; ok {
				strValue := val.(string)
				durationValue, err := time.ParseDuration(strValue)
				if err != nil {
					return errors.Errorf("error parsing [edge.oidc.accessTokenDuration], invalid duration string %s, cannot parse as duration (e.g. 1m): %v", strValue, err)
				}

				if durationValue < 1*time.Minute {
					pfxlog.Logger().Warn("field [edge.oidc.accessTokenDuration] is too short, setting to 1m")
					durationValue = 1 * time.Minute
				}

				c.Oidc.AccessTokenDuration = durationValue
			}

			if val, ok := oidcSubMap["idTokenDuration"]; ok {
				strValue := val.(string)
				durationValue, err := time.ParseDuration(strValue)
				if err != nil {
					return errors.Errorf("error parsing [edge.oidc.idTokenDuration], invalid duration string %s, cannot parse as duration (e.g. 1m): %v", strValue, err)
				}

				if durationValue < 1*time.Minute {
					pfxlog.Logger().Warn("field [edge.oidc.idTokenDuration] is too short, setting to 1m")
					durationValue = 1 * time.Minute
				}

				c.Oidc.IdTokenDuration = durationValue
			}

			if val, ok := oidcSubMap["refreshTokenDuration"]; ok {
				strValue := val.(string)
				durationValue, err := time.ParseDuration(strValue)
				if err != nil {
					return errors.Errorf("error parsing [edge.oidc.refreshTokenDuration], invalid duration string %s, cannot parse as duration (e.g. 1m): %v", strValue, err)
				}

				if durationValue-1*time.Minute < c.Oidc.AccessTokenDuration {
					newVal := c.Oidc.AccessTokenDuration + 1 + time.Minute
					pfxlog.Logger().Warnf("field [edge.oidc.refreshTokenDuration] is too short [%s], must be larger than access token duration by 1 minute, setting to "+newVal.String(), durationValue.String())
					durationValue = newVal
				}

				c.Oidc.RefreshTokenDuration = durationValue
			}

			if val, ok := oidcSubMap["revocationMinTokenLifetime"]; ok {
				strValue := val.(string)
				durationValue, err := time.ParseDuration(strValue)
				if err != nil {
					return errors.Errorf("error parsing [edge.oidc.revocationMinTokenLifetime], invalid duration string %s: %v", strValue, err)
				}
				c.Oidc.RevocationMinTokenLifetime = durationValue
			}

			if val, ok := oidcSubMap["revocationBucketInterval"]; ok {
				strValue := val.(string)
				durationValue, err := time.ParseDuration(strValue)
				if err != nil {
					return errors.Errorf("error parsing [edge.oidc.revocationBucketInterval], invalid duration string %s: %v", strValue, err)
				}
				if durationValue < 1*time.Second {
					pfxlog.Logger().Warn("field [edge.oidc.revocationBucketInterval] is too short, setting to 1s")
					durationValue = 1 * time.Second
				}
				c.Oidc.RevocationBucketInterval = durationValue
			}

			if val, ok := oidcSubMap["revocationBucketMaxSize"]; ok {
				c.Oidc.RevocationBucketMaxSize = val.(int)
			}

			if val, ok := oidcSubMap["revocationMaxQueued"]; ok {
				c.Oidc.RevocationMaxQueued = val.(int)
			}

			if val, ok := oidcSubMap["revocationEnforcerFrequency"]; ok {
				strValue := val.(string)
				durationValue, err := time.ParseDuration(strValue)
				if err != nil {
					return errors.Errorf("error parsing [edge.oidc.revocationEnforcerFrequency], invalid duration string %s: %v", strValue, err)
				}
				if durationValue < 10*time.Second {
					pfxlog.Logger().Warn("field [edge.oidc.revocationEnforcerFrequency] is too short, setting to 10s")
					durationValue = 10 * time.Second
				}
				c.Oidc.RevocationEnforcerFrequency = durationValue
			}
		}
	}

	if c.Oidc.RevocationMinTokenLifetime > 0 {
		limit := c.Oidc.RefreshTokenDuration / 2
		if c.Oidc.RevocationMinTokenLifetime >= limit {
			return errors.Errorf("[edge.oidc.revocationMinTokenLifetime] %s must be less than 50%% of refreshTokenDuration %s",
				c.Oidc.RevocationMinTokenLifetime, c.Oidc.RefreshTokenDuration)
		}
	}

	return nil
}

func (c *EdgeConfig) loadApiSection(edgeConfigMap map[interface{}]interface{}) error {
	c.Api = Api{}
	c.Api.HttpTimeouts = *DefaultHttpTimeouts()
	var err error

	c.Api.ActivityUpdateBatchSize = DefaultEdgeApiActivityUpdateBatchSize
	c.Api.ActivityUpdateInterval = DefaultEdgeAPIActivityUpdateInterval

	if value, found := edgeConfigMap["api"]; found {
		apiSubMap := value.(map[interface{}]interface{})

		if val, ok := apiSubMap["address"]; ok {
			if c.Api.Address, ok = val.(string); !ok {
				return errors.Errorf("invalid type %t for [edge.api.address], must be string", val)
			}

			if c.Api.Address == "" {
				return errors.Errorf("invalid type %t for [edge.api.address], must not be an empty string", val)
			}

			if err := validateHostPortString(c.Api.Address); err != nil {
				return errors.Errorf("invalid value %s for [edge.api.address]: %v", c.Api.Address, err)
			}
		} else {
			return errors.New("required value [edge.api.address] is required")
		}

		var durationValue = 0 * time.Second
		if value, found := apiSubMap["sessionTimeout"]; found {
			strValue := value.(string)
			durationValue, err = time.ParseDuration(strValue)
			if err != nil {
				return errors.Errorf("error parsing [edge.api.sessionTimeout], invalid duration string %s, cannot parse as duration (e.g. 1m): %v", strValue, err)
			}
		}

		if durationValue < MinEdgeSessionTimeout {
			durationValue = DefaultEdgeSessionTimeout
			pfxlog.Logger().Warnf("[edge.api.sessionTimeout] defaulted to %v", durationValue)
		}

		c.Api.SessionTimeout = durationValue

		if val, ok := apiSubMap["activityUpdateBatchSize"]; ok {
			if c.Api.ActivityUpdateBatchSize, ok = val.(int); !ok {
				return errors.Errorf("invalid type %v for apiSessions.activityUpdateBatchSize, must be int", reflect.TypeOf(val))
			}
		}

		if val, ok := apiSubMap["activityUpdateInterval"]; ok {
			if strVal, ok := val.(string); !ok {
				return errors.Errorf("invalid type %v for apiSessions.activityUpdateInterval, must be string duration", reflect.TypeOf(val))
			} else {
				if c.Api.ActivityUpdateInterval, err = time.ParseDuration(strVal); err != nil {
					return errors.Wrapf(err, "invalid value %v for apiSessions.activityUpdateInterval, must be string duration", val)
				}
			}
		}

		if c.Api.ActivityUpdateBatchSize < MinEdgeAPIActivityUpdateBatchSize || c.Api.ActivityUpdateBatchSize > MaxEdgeAPIActivityUpdateBatchSize {
			return errors.Errorf("invalid value %v for apiSessions.activityUpdateBatchSize, must be between %v and %v", c.Api.ActivityUpdateBatchSize, MinEdgeAPIActivityUpdateBatchSize, MaxEdgeAPIActivityUpdateBatchSize)
		}

		if c.Api.ActivityUpdateInterval < MinEdgeAPIActivityUpdateInterval || c.Api.ActivityUpdateInterval > MaxEdgeAPIActivityUpdateInterval {
			return errors.Errorf("invalid value %v for apiSessions.activityUpdateInterval, must be between %vms and %vm", c.Api.ActivityUpdateInterval.String(), MinEdgeAPIActivityUpdateInterval.Milliseconds(), MaxEdgeAPIActivityUpdateInterval.Minutes())
		}

		if v, ok := apiSubMap["disableOidcAutoBinding"]; ok {
			if boolVal, ok := v.(bool); ok {
				c.Api.DisableOidcAutoBinding = boolVal
			} else if strVal, ok := v.(string); ok {
				c.Api.DisableOidcAutoBinding = strings.EqualFold("true", strVal)
			} else {
				return errors.Errorf("invalid type for 'disableOidcAutoBinding' config %T, expected bool or string (\"true\"/\"false\")", v)
			}
		}

		return nil

	} else {
		return errors.New("required configuration section [edge.api] missing")
	}
}

func validateHostPortString(address string) error {
	address = strings.TrimSpace(address)

	if address == "" {
		return errors.New("must not be an empty string or unspecified")
	}

	host, port, err := net.SplitHostPort(address)

	if err != nil {
		return errors.Errorf("could not split host and port: %v", err)
	}

	if host == "" {
		return errors.New("host must be specified")
	}

	if port == "" {
		return errors.New("port must be specified")
	}

	if port, err := strconv.ParseInt(port, 10, 32); err != nil {
		return errors.New("invalid port, must be a integer")
	} else if port < 1 || port > 65535 {
		return errors.New("invalid port, must 1-65535")
	}

	return nil
}

func (c *EdgeConfig) loadEnrollmentSection(edgeConfigMap map[interface{}]interface{}) error {
	c.Enrollment = Enrollment{}
	var err error

	if value, found := edgeConfigMap["enrollment"]; found {
		enrollmentSubMap := value.(map[interface{}]interface{})

		if value, found := enrollmentSubMap["signingCert"]; found {
			signingCertSubMap := value.(map[interface{}]interface{})
			c.Enrollment.SigningCertConfig = identity.Config{}

			if value, found := signingCertSubMap["cert"]; found {
				c.Enrollment.SigningCertConfig.Cert = value.(string)
				certPem, err := os.ReadFile(c.Enrollment.SigningCertConfig.Cert)
				if err != nil {
					pfxlog.Logger().WithError(err).Panic("unable to read [edge.enrollment.cert]")
				}
				//The signer is a valid trust anchor
				_, _ = c.caPems.WriteString("\n")
				_, _ = c.caPems.Write(certPem)

			} else {
				return fmt.Errorf("required configuration value [edge.enrollment.cert] is missing")
			}

			if value, found := signingCertSubMap["key"]; found {
				c.Enrollment.SigningCertConfig.Key = value.(string)
			} else {
				return fmt.Errorf("required configuration value [edge.enrollment.key] is missing")
			}

			if value, found := signingCertSubMap["ca"]; found {
				c.Enrollment.SigningCertConfig.CA = value.(string)

				if c.Enrollment.SigningCertCaPem, err = os.ReadFile(c.Enrollment.SigningCertConfig.CA); err != nil {
					return fmt.Errorf("could not read file CA file from [edge.enrollment.signingCert.ca]")
				}

				_, _ = c.caPems.WriteString("\n")
				_, _ = c.caPems.Write(c.Enrollment.SigningCertCaPem)
			} //not an error if the signing certificate's CA is already represented in the root [identity.ca]

			if c.Enrollment.SigningCert, err = identity.LoadIdentity(c.Enrollment.SigningCertConfig); err != nil {
				return fmt.Errorf("error loading [edge.enrollment.signingCert]: %s", err)
			} else {
				if err := c.Enrollment.SigningCert.WatchFiles(); err != nil {
					pfxlog.Logger().Warn("could not enable file watching on enrollment signing cert: %w", err)
				}
			}

		} else {
			return errors.New("required configuration section [edge.enrollment.signingCert] missing")
		}

		if value, found := enrollmentSubMap["edgeIdentity"]; found {
			edgeIdentitySubMap := value.(map[interface{}]interface{})

			edgeIdentityDuration := 0 * time.Second
			if value, found := edgeIdentitySubMap["duration"]; found {
				strValue := value.(string)
				var err error
				edgeIdentityDuration, err = time.ParseDuration(strValue)

				if err != nil {
					return errors.Errorf("error parsing [edge.enrollment.edgeIdentity.duration], invalid duration string %s, cannot parse as duration (e.g. 1m): %v", strValue, err)
				}
			}

			if edgeIdentityDuration < MinEdgeEnrollmentDuration {
				edgeIdentityDuration = DefaultEdgeEnrollmentDuration
			}

			c.Enrollment.EdgeIdentity = EnrollmentOption{Duration: edgeIdentityDuration}

		} else {
			return errors.New("required configuration section [edge.enrollment.edgeIdentity] missing")
		}

		if value, found := enrollmentSubMap["edgeRouter"]; found {
			edgeRouterSubMap := value.(map[interface{}]interface{})

			edgeRouterDuration := 0 * time.Second
			if value, found := edgeRouterSubMap["duration"]; found {
				strValue := value.(string)
				var err error
				edgeRouterDuration, err = time.ParseDuration(strValue)

				if err != nil {
					return errors.Errorf("error parsing [edge.enrollment.edgeRouter.duration], invalid duration string %s, cannot parse as duration (e.g. 1m): %v", strValue, err)
				}
			}

			if edgeRouterDuration < MinEdgeEnrollmentDuration {
				edgeRouterDuration = DefaultEdgeEnrollmentDuration
			}

			c.Enrollment.EdgeRouter = EnrollmentOption{Duration: edgeRouterDuration}

		} else {
			return errors.New("required configuration section [edge.enrollment.edgeRouter] missing")
		}

	} else {
		return errors.New("required configuration section [edge.enrollment] missing")
	}

	return nil
}

func (c *EdgeConfig) loadAuthRateLimiterConfig(cfgmap map[interface{}]interface{}) error {
	c.AuthRateLimiter.SetDefaults()

	c.AuthRateLimiter.Enabled = DefaultAuthRateLimiterEnabled
	c.AuthRateLimiter.MaxSize = DefaultAuthRateLimiterMaxSize
	c.AuthRateLimiter.MinSize = DefaultAuthRateLimiterMinSize

	if value, found := cfgmap["authRateLimiter"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if err := c.AuthRateLimiter.Load(submap); err != nil {
				return err
			}
			if c.AuthRateLimiter.MaxSize < AuthRateLimiterMinSizeValue {
				return errors.Errorf("invalid value %v for authRateLimiter.maxSize, must be at least %v",
					c.AuthRateLimiter.MaxSize, AuthRateLimiterMinSizeValue)
			}
			if c.AuthRateLimiter.MaxSize > AuthRateLimiterMaxSizeValue {
				return errors.Errorf("invalid value %v for authRateLimiter.maxSize, must be at most %v",
					c.AuthRateLimiter.MaxSize, AuthRateLimiterMaxSizeValue)
			}

			if c.AuthRateLimiter.MinSize < AuthRateLimiterMinSizeValue {
				return errors.Errorf("invalid value %v for authRateLimiter.minSize, must be at least %v",
					c.AuthRateLimiter.MinSize, AuthRateLimiterMinSizeValue)
			}
			if c.AuthRateLimiter.MinSize > AuthRateLimiterMaxSizeValue {
				return errors.Errorf("invalid value %v for authRateLimiter.minSize, must be at most %v",
					c.AuthRateLimiter.MinSize, AuthRateLimiterMaxSizeValue)
			}
		} else {
			return errors.Errorf("invalid type for authRateLimiter, should be map instead of %T", value)
		}
	}

	return nil
}

func (c *EdgeConfig) loadIdentityStatusConfig(cfgmap map[interface{}]interface{}) error {
	c.IdentityStatusConfig.ScanInterval = DefaultIdentityOnlineStatusScanInterval
	c.IdentityStatusConfig.UnknownTimeout = DefaultIdentityOnlineStatusUnknownTimeout
	c.IdentityStatusConfig.Source = DefaultIdentityOnlineStatusSource

	if value, found := cfgmap["identityStatusConfig"]; found {
		if submap, ok := value.(map[interface{}]interface{}); ok {
			if value, found := submap["scanInterval"]; found {
				if interval, err := time.ParseDuration(fmt.Sprintf("%v", value)); err != nil {
					pfxlog.Logger().WithError(err).Errorf("invalid value '%v' for identity status config scan interval", value)
				} else {
					c.IdentityStatusConfig.ScanInterval = interval
				}
			}

			if c.IdentityStatusConfig.ScanInterval < MinIdentityOnlineStatusScanInterval {
				pfxlog.Logger().Errorf("invalid value '%v' for identity status config scan interval, must be at least %s",
					c.IdentityStatusConfig.ScanInterval, MinIdentityOnlineStatusScanInterval)
				c.IdentityStatusConfig.ScanInterval = MinIdentityOnlineStatusScanInterval
			}

			if value, found := submap["unknownTimeout"]; found {
				if interval, err := time.ParseDuration(fmt.Sprintf("%v", value)); err != nil {
					pfxlog.Logger().WithError(err).Errorf("invalid value '%v' for identity status config unknown timeout", value)
				} else {
					c.IdentityStatusConfig.UnknownTimeout = interval
				}
			}

			if value, found := submap["source"]; found {
				strVal := fmt.Sprintf("%v", value)
				switch strVal {
				case "heartbeats":
					c.IdentityStatusConfig.Source = IdentityStatusSourceHeartbeats
				case "connect-events":
					c.IdentityStatusConfig.Source = IdentityStatusSourceConnectEvents
				case "hybrid":
					c.IdentityStatusConfig.Source = IdentityStatusSourceHybrid
				default:
					pfxlog.Logger().Errorf("invalid value '%v' for identity status config source, valid values: ['heartbeats', 'connect-events', 'hybrid']", strVal)
				}
			}

		} else {
			return errors.Errorf("invalid type for identityStatusConfig, should be map instead of %T", value)
		}
	}

	return nil
}

// loadExternalJwtSignersSection loads [edge.externalJwtSigners]. Every value is optional;
// absent values keep the defaults from DefaultJwksFetch.
func (c *EdgeConfig) loadExternalJwtSignersSection(edgeConfigMap map[any]any) error {
	c.ExternalJwtSigners.JwksFetch = DefaultJwksFetch()

	value, found := edgeConfigMap["externalJwtSigners"]

	if !found || value == nil {
		return nil
	}

	extJwtSignersMap, ok := value.(map[any]any)

	if !ok {
		return errors.Errorf("invalid type %T for [edge.externalJwtSigners], must be a map", value)
	}

	value, found = extJwtSignersMap["jwksFetch"]

	if !found || value == nil {
		return nil
	}

	jwksFetchMap, ok := value.(map[any]any)

	if !ok {
		return errors.Errorf("invalid type %T for [edge.externalJwtSigners.jwksFetch], must be a map", value)
	}

	jwksFetch := &c.ExternalJwtSigners.JwksFetch

	if val, found := jwksFetchMap["blockPrivateAddresses"]; found && val != nil {
		switch typedVal := val.(type) {
		case bool:
			jwksFetch.BlockPrivateAddresses = typedVal
		case string:
			boolVal, err := strconv.ParseBool(typedVal)
			if err != nil {
				return errors.Errorf("invalid value %q for [edge.externalJwtSigners.jwksFetch.blockPrivateAddresses], must be a boolean", typedVal)
			}
			jwksFetch.BlockPrivateAddresses = boolVal
		default:
			return errors.Errorf("invalid type %T for [edge.externalJwtSigners.jwksFetch.blockPrivateAddresses], must be a boolean", val)
		}
	}

	if val, found := jwksFetchMap["deniedIPs"]; found && val != nil {
		addresses, err := parseCidrList(val, "edge.externalJwtSigners.jwksFetch.deniedIPs")
		if err != nil {
			return err
		}
		jwksFetch.DeniedIPs = addresses
	}

	if val, found := jwksFetchMap["allowedIPs"]; found && val != nil {
		addresses, err := parseCidrList(val, "edge.externalJwtSigners.jwksFetch.allowedIPs")
		if err != nil {
			return err
		}
		jwksFetch.AllowedIPs = addresses
	}

	if val, found := jwksFetchMap["deniedHostnames"]; found && val != nil {
		hosts, err := parseHostnameList(val, "edge.externalJwtSigners.jwksFetch.deniedHostnames")
		if err != nil {
			return err
		}
		jwksFetch.DeniedHostnames = hosts
	}

	if val, found := jwksFetchMap["allowedHostnames"]; found && val != nil {
		hosts, err := parseHostnameList(val, "edge.externalJwtSigners.jwksFetch.allowedHostnames")
		if err != nil {
			return err
		}
		jwksFetch.AllowedHostnames = hosts
	}

	if val, found := jwksFetchMap["timeout"]; found && val != nil {
		strVal, ok := val.(string)

		if !ok {
			return errors.Errorf("invalid type %T for [edge.externalJwtSigners.jwksFetch.timeout], must be a string duration", val)
		}

		durationVal, err := time.ParseDuration(strVal)

		if err != nil {
			return errors.Errorf("error parsing [edge.externalJwtSigners.jwksFetch.timeout], invalid duration string %s, cannot parse as duration (e.g. 5s): %v", strVal, err)
		}

		if durationVal <= 0 {
			return errors.Errorf("invalid value %s for [edge.externalJwtSigners.jwksFetch.timeout], must be greater than zero", strVal)
		}

		jwksFetch.Timeout = durationVal
	}

	if val, found := jwksFetchMap["maxRedirects"]; found && val != nil {
		intVal, ok := val.(int)

		if !ok {
			return errors.Errorf("invalid type %T for [edge.externalJwtSigners.jwksFetch.maxRedirects], must be an integer", val)
		}

		if intVal < 0 {
			return errors.Errorf("invalid value %v for [edge.externalJwtSigners.jwksFetch.maxRedirects], must not be negative", intVal)
		}

		jwksFetch.MaxRedirects = intVal
	}

	return nil
}

// parseCidrList parses a list of CIDRs, accepting a bare IP address as a single address CIDR
// (/32 for IPv4, /128 for IPv6). Hostnames are rejected: the address check happens at dial
// time against the resolved IP, so a hostname entry could never be matched reliably.
func parseCidrList(value any, field string) ([]*net.IPNet, error) {
	values, ok := value.([]any)

	if !ok {
		return nil, errors.Errorf("invalid type %T for [%s], must be a list of CIDRs", value, field)
	}

	var result []*net.IPNet

	for _, entry := range values {
		strVal, ok := entry.(string)

		if !ok {
			return nil, errors.Errorf("invalid type %T for an entry in [%s], must be a string CIDR", entry, field)
		}

		ipNet, err := parseCidrOrIp(strings.TrimSpace(strVal))

		if err != nil {
			return nil, errors.Errorf("invalid value %q in [%s]: %v", strVal, field, err)
		}

		result = append(result, ipNet)
	}

	return result, nil
}

// parseHostnameList parses a list of hostname patterns, returning them normalized for comparison
// against a URL's hostname.
func parseHostnameList(value any, field string) ([]string, error) {
	values, ok := value.([]any)

	if !ok {
		return nil, errors.Errorf("invalid type %T for [%s], must be a list of hostnames", value, field)
	}

	var result []string

	for _, entry := range values {
		strVal, ok := entry.(string)

		if !ok {
			return nil, errors.Errorf("invalid type %T for an entry in [%s], must be a string hostname", entry, field)
		}

		pattern, err := parseHostnamePattern(strings.TrimSpace(strVal))

		if err != nil {
			return nil, errors.Errorf("invalid value %q in [%s]: %v", strVal, field, err)
		}

		result = append(result, pattern)
	}

	return result, nil
}

// parseHostnamePattern validates a host entry and returns it normalized. An entry is either an
// exact host (idp.example.com) or a wildcard suffix (*.example.com), which matches any
// subdomain of that suffix but not the suffix itself. IP addresses are rejected: matching an
// address by name comparison would not be an address check.
func parseHostnamePattern(value string) (string, error) {
	if value == "" {
		return "", errors.New("must not be empty")
	}

	host := strings.TrimPrefix(value, "*.")
	isWildcard := host != value

	if net.ParseIP(strings.Trim(host, "[]")) != nil {
		return "", errors.New("must be a hostname, use deniedIPs or allowedIPs for IP addresses")
	}

	if host == "" {
		return "", errors.New("must include a hostname after the leading \"*.\"")
	}

	if strings.ContainsAny(host, "*/:@ \t") {
		return "", errors.New("must be a bare host name, without a scheme, port or path, and a wildcard is only supported as a leading \"*.\"")
	}

	normalized := NormalizeHostname(host)

	if normalized == "" {
		return "", errors.New("must be a valid host name")
	}

	if isWildcard {
		return "*." + normalized, nil
	}

	return normalized, nil
}

// NormalizeHostname returns a host in the form used for comparison: lower-cased, without a
// trailing dot, and converted to punycode when it contains non-ASCII labels. Config entries
// and request hosts are both normalized this way so that they compare consistently.
func NormalizeHostname(host string) string {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")

	if ascii, err := idna.Lookup.ToASCII(host); err == nil {
		host = ascii
	}

	return strings.ToLower(host)
}

// parseCidrOrIp parses a CIDR or a bare IP address into a *net.IPNet. A bare IP address
// becomes a single address CIDR.
func parseCidrOrIp(value string) (*net.IPNet, error) {
	if _, ipNet, err := net.ParseCIDR(value); err == nil {
		return ipNet, nil
	}

	if ip := net.ParseIP(value); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			return &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}, nil
		}

		return &net.IPNet{IP: ip.To16(), Mask: net.CIDRMask(128, 128)}, nil
	}

	return nil, errors.New("must be a CIDR (e.g. 10.0.0.0/8) or an IP address, hostnames are not supported")
}

func LoadEdgeConfigFromMap(configMap map[interface{}]interface{}) (*EdgeConfig, error) {
	edgeConfig := NewEdgeConfig()

	var edgeConfigMap map[interface{}]interface{}

	if val, ok := configMap["edge"]; ok && val != nil {
		if edgeConfigMap, ok = val.(map[interface{}]interface{}); !ok {
			return nil, fmt.Errorf("expected map as edge configuration")
		}
	} else {
		return edgeConfig, nil
	}

	edgeConfig.Enabled = configMap != nil

	if !edgeConfig.Enabled {
		return edgeConfig, nil
	}

	var err error

	if err = edgeConfig.loadApiSection(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadOidcSection(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadTotpSection(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadEnrollmentSection(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadAuthRateLimiterConfig(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadIdentityStatusConfig(edgeConfigMap); err != nil {
		return nil, err
	}

	if err = edgeConfig.loadExternalJwtSignersSection(edgeConfigMap); err != nil {
		return nil, err
	}

	if v, ok := edgeConfigMap["disablePostureChecks"]; ok {
		if boolVal, ok := v.(bool); ok {
			edgeConfig.DisablePostureChecks = boolVal
		} else if strVal, ok := v.(string); ok {
			edgeConfig.DisablePostureChecks = strings.EqualFold("true", strVal)
		} else {
			return nil, fmt.Errorf("invalid type for 'enablePostureChecks' config %T", v)
		}
	}

	return edgeConfig, nil
}

// CalculateCaPems takes the supplied caPems buffer as a set of PEM Certificates separated by new lines. Duplicate
// certificates are removed, and the result is returned as a bytes.Buffer of PEM Certificates separated by new lines.
func CalculateCaPems(caPems *bytes.Buffer) *bytes.Buffer {
	caPemMap := map[string][]byte{}

	newCaPems := bytes.Buffer{}
	blocksToProcess := caPems.Bytes()

	for len(blocksToProcess) != 0 {
		var block *pem.Block
		block, blocksToProcess = pem.Decode(blocksToProcess)

		if block != nil {

			if block.Type != "CERTIFICATE" {
				pfxlog.Logger().
					WithField("type", block.Type).
					WithField("block", string(pem.EncodeToMemory(block))).
					Warn("encountered an invalid PEM block type loading configured CAs, block will be ignored")
				continue
			}

			cert, err := x509.ParseCertificate(block.Bytes)

			if err != nil {
				pfxlog.Logger().
					WithField("type", block.Type).
					WithField("block", string(pem.EncodeToMemory(block))).
					WithError(err).
					Warn("block could not be parsed as a certificate, block will be ignored")
				continue
			}

			if !cert.IsCA {
				pfxlog.Logger().
					WithField("type", block.Type).
					WithField("block", string(pem.EncodeToMemory(block))).
					Warn("block is not a CA, block will be ignored")
				continue
			}
			// #nosec
			hash := sha1.Sum(block.Bytes)
			fingerprint := toHex(hash[:])
			newPem := pem.EncodeToMemory(block)
			caPemMap[fingerprint] = newPem
		} else {
			blocksToProcess = nil
		}
	}

	for _, caPem := range caPemMap {
		_, _ = newCaPems.WriteString("\n")
		_, _ = newCaPems.Write(caPem)
	}

	return &newCaPems
}

// toHex takes a byte array returns a hex formatted fingerprint
func toHex(data []byte) string {
	var buf bytes.Buffer
	for i, b := range data {
		if i > 0 {
			_, _ = fmt.Fprintf(&buf, ":")
		}
		_, _ = fmt.Fprintf(&buf, "%02x", b)
	}
	return strings.ToUpper(buf.String())
}
