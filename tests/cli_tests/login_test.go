//go:build cli_tests

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
package cli_tests

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/ziti/cmd"
	"github.com/openziti/ziti/v2/ziti/cmd/edge"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func (s *cliTestState) loginTests(t *testing.T) {
	t.Run("correct password succeeds", s.testCorrectPasswordSucceeds)
	t.Run("wrong password fails", s.testWrongPasswordFails)
	t.Run("token based login", s.testTokenBasedLogin)
	t.Run("client cert authentication - no ca", s.testClientCertAuthentication)
	t.Run("identity file authentication", s.testIdentityFileAuthentication)
	t.Run("identity file authentication - ctrl url unset", s.testIdentityFileAuthenticationCtrlUrlUnset)
	t.Run("identity file auth then cached request", s.testIdentityFileLoginThenCachedRequest)
	t.Run("identity file auth then separate process", s.testIdentityFileLoginThenSeparateProcess)
	t.Run("client cert auth then cached request", s.testClientCertLoginThenCachedRequest)
	t.Run("identity file given as a relative path", s.testIdentityFileRelativePath)
	t.Run("identity file auth then token refresh", s.testIdentityFileLoginThenTokenRefresh)
	t.Run("external JWT authentication", s.testExternalJWTAuthentication)
	t.Run("network identity zitified connection", s.testNetworkIdentityZitifiedConnection)

	// Certificate caching and controller switching when logging in
	t.Run("switch controller different cert", s.testSwitchController)
	t.Run("same controller uses cached cert", s.testSameControllerUsesCache)
	t.Run("switch controller file auth", s.testSwitchControllerFileAuth)
	t.Run("file auth ignores cached server cert", s.testFileAuthIgnoresCachedServerCert)
	t.Run("os-trusted server cert uses system store and clears cache", s.testTrustedServerCertUsesSystemStore)

	// Edge Cases
	t.Run("empty username", s.testEmptyUsername)
	t.Run("empty password", s.testEmptyPassword)
	t.Run("invalid controller URL", s.testInvalidControllerURL)
	t.Run("non-existent username", s.testNonExistentUsername)
	t.Run("controller unavailable", s.testControllerUnavailable)
}

// Authentication Methods
func (s *cliTestState) testCorrectPasswordSucceeds(t *testing.T) {
	s.removeZitiDir(t)
	opts := s.controllerUnderTest.NewTestLoginOpts()

	err := opts.Run()
	require.NoError(t, err)
	require.NotEmpty(t, opts.ApiSession)
	t.Logf("Login successful to %s, token: %s", s.controllerUnderTest.ControllerHostPort(), opts.ApiSession.GetToken())

	// Verify we can create a management client
	client, err := opts.NewManagementClient(false)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotEmpty(t, opts.ApiSession)
}

func (s *cliTestState) testWrongPasswordFails(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.controllerUnderTest.Username,
		Password:      "wrong-password",
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "login with wrong password should fail")
}

func (s *cliTestState) testTokenBasedLogin(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		ApiSession:    s.controllerUnderTest.ApiSession,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.NoError(t, err)
	require.NotEmpty(t, opts.ApiSession)
	require.NotEmpty(t, opts.ApiSession.GetToken())
	t.Logf("Login successful, token: %s", opts.ApiSession.GetToken())
}

func (s *cliTestState) testClientCertAuthentication(t *testing.T) {
	// Setup common options
	baseOpts := edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		ClientCert:    s.controllerUnderTest.AdminCertFile,
		ClientKey:     s.controllerUnderTest.AdminKeyFile,
		CaCert:        s.controllerUnderTest.AdminCaFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	s.removeZitiDir(t)
	t.Run("all present", func(t *testing.T) {
		opts := baseOpts

		err := opts.Run()
		require.NoError(t, err, "login with cert/key/ca when all present should succeed")
		require.NotEmpty(t, opts.ApiSession)
		t.Logf("Login successful, token: %s", opts.Token)
	})

	s.removeZitiDir(t)
	t.Run("no cert", func(t *testing.T) {
		opts := baseOpts
		opts.ClientCert = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client cert is missing")
		require.Contains(t, err.Error(), "username required but not provided")
	})

	s.removeZitiDir(t)
	t.Run("no key", func(t *testing.T) {
		opts := baseOpts
		opts.ClientKey = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client key is missing")
		require.Contains(t, err.Error(), "failed to read key")
	})

	s.removeZitiDir(t)
	t.Run("no CA cert with yes flag", func(t *testing.T) {
		opts := baseOpts
		opts.CaCert = ""
		opts.Yes = true

		err := opts.Run()
		require.NoError(t, err, "expected success when CA cert is missing and IgnoreConfig is enabled and 'Yes' is true")
		require.NotEmpty(t, opts.ApiSession)
		t.Logf("Login successful, token: %s", opts.Token)
	})

	s.removeZitiDir(t)
	t.Run("no CA cert without yes flag", func(t *testing.T) {
		opts := baseOpts
		opts.CaCert = ""
		opts.Yes = false

		err := opts.Run()
		require.Error(t, err, "expected error when CA cert is missing")
		require.Contains(t, err.Error(), "Cannot accept certs - no terminal")
	})
}

func (s *cliTestState) testIdentityFileAuthenticationCtrlUrlUnset(t *testing.T) {
	// tests that the file supplied provides the proper url for login
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: "",
		Yes:           true,
		IgnoreConfig:  true,
		File:          s.controllerUnderTest.AdminIdFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}
	t.Log("FILE: ", s.controllerUnderTest.AdminIdFile)
	err := opts.Run()
	require.NoError(t, err)
	client, err := opts.NewManagementClient(false)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotEmpty(t, opts.ApiSession)
	t.Logf("Login successful, token: %s", opts.Token)
}

func (s *cliTestState) testIdentityFileAuthentication(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		File:          s.controllerUnderTest.AdminIdFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.NoError(t, err)

	client, err := opts.NewManagementClient(false)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotEmpty(t, opts.ApiSession)
	t.Logf("Login successful for %s, token: %s", s.controllerUnderTest.ControllerHostPort(), opts.Token)
}

// The tests above stop at "login succeeded". The ones below carry on to the part that matters: a command
// issued afterwards, which loads the identity back off disk and builds a new client from it. Certificate
// authentication binds the session to the client certificate, so that new client has to present the
// certificate again or the controller rejects the session.

// testIdentityFileLoginThenCachedRequest logs in with an identity file, then makes a request the way a
// later command does -- through the identity cached in ZITI_CONFIG_DIR rather than through the
// LoginOptions still held in memory.
func (s *cliTestState) testIdentityFileLoginThenCachedRequest(t *testing.T) {
	s.removeZitiDir(t)

	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false, // the cached identity is the thing under test
		File:          s.controllerUnderTest.AdminIdFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}
	require.NoError(t, opts.Run(), "file-based login should succeed")
	require.NotEmpty(t, opts.ApiSession)
	util.ReloadConfig() // drop the memoized identity so the cached one is read back off disk

	// `ziti edge list identities` lands here: load the cached identity, build a client from it, replay
	// the saved session.
	_, err := util.EdgeControllerList("identities", nil, false, os.Stdout, 10, false)
	require.NoError(t, err, "a command run after 'ziti edge login -f' must still be authorized")
}

// testClientCertLoginThenCachedRequest is the same check for --client-cert/--client-key, which caches the
// same way and so has the same exposure.
func (s *cliTestState) testClientCertLoginThenCachedRequest(t *testing.T) {
	s.removeZitiDir(t)

	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		ClientCert:    s.controllerUnderTest.AdminCertFile,
		ClientKey:     s.controllerUnderTest.AdminKeyFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}
	require.NoError(t, opts.Run(), "client cert login should succeed")
	require.NotEmpty(t, opts.ApiSession)
	util.ReloadConfig() // drop the memoized identity so the cached one is read back off disk

	_, err := util.EdgeControllerList("identities", nil, false, os.Stdout, 10, false)
	require.NoError(t, err, "a command run after 'ziti edge login --client-cert' must still be authorized")
}

// testIdentityFileLoginThenSeparateProcess is the same sequence with a real process boundary, so nothing
// in memory can carry the certificate across for it.
func (s *cliTestState) testIdentityFileLoginThenSeparateProcess(t *testing.T) {
	s.removeZitiDir(t)

	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		File:          s.controllerUnderTest.AdminIdFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}
	require.NoError(t, opts.Run(), "file-based login should succeed")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// ZITI_CONFIG_DIR is already set for the suite, so the child reads the identity this login just wrote.
	cmd := exec.CommandContext(ctx, s.zitiPath, "edge", "list", "identities")
	out, err := cmd.CombinedOutput()
	t.Logf("ziti edge list identities:\n%s", strings.TrimSpace(string(out)))
	require.NoError(t, err, "'ziti edge list identities' after 'ziti edge login -f' must still be authorized")
}

// testIdentityFileRelativePath logs in with a relative path and then runs a command from a different
// working directory. The cached identity has to hold a path that still resolves from anywhere.
func (s *cliTestState) testIdentityFileRelativePath(t *testing.T) {
	s.removeZitiDir(t)

	idDir := filepath.Dir(s.controllerUnderTest.AdminIdFile)
	relativeId := "." + string(filepath.Separator) + filepath.Base(s.controllerUnderTest.AdminIdFile)

	startDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(idDir), "log in from the directory holding the identity file")
	defer func() { _ = os.Chdir(startDir) }()

	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		File:          relativeId,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}
	require.NoError(t, opts.Run(), "login with a relative identity file should succeed")

	// move away, exactly as a user would by cd-ing elsewhere before the next command
	require.NoError(t, os.Chdir(startDir))
	util.ReloadConfig() // drop the memoized identity so the cached one is read back off disk

	_, err = util.EdgeControllerList("identities", nil, false, os.Stdout, 10, false)
	require.NoError(t, err, "the cached identity must resolve from a different working directory")
}

// testIdentityFileLoginThenTokenRefresh covers the delayed version of the same failure. Once the access
// token expires the CLI refreshes it from the cached refresh token, and the controller verifies the
// certificate binding on that exchange too, so the refresh has to present the certificate as well.
// Rather than wait out the token lifetime, rewrite the cached access token so it has already expired.
func (s *cliTestState) testIdentityFileLoginThenTokenRefresh(t *testing.T) {
	s.removeZitiDir(t)

	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		File:          s.controllerUnderTest.AdminIdFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}
	require.NoError(t, opts.Run(), "file-based login should succeed")

	expireCachedAccessToken(t)
	util.ReloadConfig() // drop the memoized identity so the expired token is read back off disk

	// this drives refreshOidcTokenIfExpired, which talks to the controller's token endpoint
	_, err := util.EdgeControllerList("identities", nil, false, os.Stdout, 10, false)
	require.NoError(t, err, "refreshing an expired cert-bound session must present the client certificate")
}

// expireCachedAccessToken rewrites the cached OIDC access token so its exp is in the past, leaving the
// refresh token alone. The CLI only parses the access token unverified to read the expiry, and the
// tampered token is never sent anywhere, so the broken signature does not matter.
func expireCachedAccessToken(t *testing.T) {
	t.Helper()

	cfgDir, err := util.ConfigDir()
	require.NoError(t, err)
	configFile := filepath.Join(cfgDir, "ziti-cli.json")

	raw, err := os.ReadFile(configFile)
	require.NoError(t, err)

	var config map[string]any
	require.NoError(t, json.Unmarshal(raw, &config))

	identities, ok := config["edgeIdentities"].(map[string]any)
	require.True(t, ok, "cached config must hold edge identities")

	found := false
	for name, entry := range identities {
		identityEntry, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		session, ok := identityEntry["apiSession"].(map[string]any)
		if !ok {
			continue
		}
		accessToken, ok := session["oidcAccessToken"].(string)
		if !ok || accessToken == "" {
			continue
		}
		require.NotEmpty(t, session["oidcRefreshToken"], "identity %s needs a refresh token to refresh with", name)
		session["oidcAccessToken"] = withExpiry(t, accessToken, time.Now().Add(-time.Hour))
		found = true
	}
	require.True(t, found, "cached config must hold an OIDC access token to expire")

	updated, err := json.MarshalIndent(config, "", "    ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configFile, updated, 0600))
}

// withExpiry rebuilds a JWT with its exp claim replaced. The signature is left as-is and becomes
// invalid, which is fine for a token only ever parsed unverified.
func withExpiry(t *testing.T, token string, exp time.Time) string {
	t.Helper()

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3, "expected a three part JWT")

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)

	var claims map[string]any
	require.NoError(t, json.Unmarshal(payload, &claims))
	claims["exp"] = exp.Unix()

	rewritten, err := json.Marshal(claims)
	require.NoError(t, err)
	parts[1] = base64.RawURLEncoding.EncodeToString(rewritten)

	return strings.Join(parts, ".")
}

func (s *cliTestState) testExternalJWTAuthentication(t *testing.T) {
	// TODO: Generate valid JWT token
	t.Skip("External JWT authentication requires JWT setup")
}

func (s *cliTestState) testNetworkIdentityZitifiedConnection(t *testing.T) {
	// TODO: Create network identity file
	t.Skip("Network identity requires identity setup")
}

// Edge Cases
func (s *cliTestState) testEmptyUsername(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      "",
		Password:      s.controllerUnderTest.Password,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "empty username should fail")
	require.Contains(t, err.Error(), "username required but not provided")
	t.Logf("Empty username correctly failed: %v", err)
}

func (s *cliTestState) testEmptyPassword(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.controllerUnderTest.Username,
		Password:      "",
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "empty password should fail")
	require.Contains(t, err.Error(), "password required but not provided")
	t.Logf("Empty password correctly failed: %v", err)
}

func (s *cliTestState) testInvalidControllerURL(t *testing.T) {
	hostErrors := []string{"i/o timeout", "no such host", "server misbehaving"}
	t.Run("not-a-url", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: s.commonOpts, Username: s.controllerUnderTest.Username, Password: s.controllerUnderTest.Password,
			ControllerUrl: "not-a-url", Yes: true, IgnoreConfig: true,
			NetworkId: s.controllerUnderTest.NetworkDialingIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.True(t,
			strings.Contains(err.Error(), hostErrors[0]) ||
				strings.Contains(err.Error(), hostErrors[1]) ||
				strings.Contains(err.Error(), hostErrors[2]) ||
				strings.Contains(err.Error(), "service 'not-a-url' not found"),
			"Error %s not contained in host errors array: %v", err.Error(), hostErrors)
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("http://[invalid", func(t *testing.T) {
		opts := &edge.LoginOptions{
			Options:       s.commonOpts,
			Username:      s.controllerUnderTest.Username,
			Password:      s.controllerUnderTest.Password,
			ControllerUrl: "http://[invalid",
			Yes:           true,
			IgnoreConfig:  true,
			NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
		}
		err := opts.Run()
		require.Error(t, err)
		invurlmsg := "invalid controller URL"
		parsemsg := "unable to parse controller url"
		require.True(t,
			strings.Contains(err.Error(), invurlmsg) ||
				strings.Contains(err.Error(), parsemsg),
			`Error %s found but expected either: %s or %s`, err.Error(), invurlmsg, parsemsg)
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("ftp://wrong-scheme.com", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: s.commonOpts, Username: s.controllerUnderTest.Username, Password: s.controllerUnderTest.Password,
			ControllerUrl: "ftp://wrong-scheme.com", Yes: true, IgnoreConfig: true,
			NetworkId: s.controllerUnderTest.NetworkDialingIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.True(t,
			strings.Contains(err.Error(), hostErrors[0]) ||
				strings.Contains(err.Error(), hostErrors[1]) ||
				strings.Contains(err.Error(), hostErrors[2]) ||
				strings.Contains(err.Error(), "service 'ftp' not found"),
			"Error %s not contained in host errors array: %v", err.Error(), hostErrors)
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("https://non-existent-host-12345.local:9999", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: s.commonOpts, Username: s.controllerUnderTest.Username, Password: s.controllerUnderTest.Password,
			ControllerUrl: "https://non-existent-host-12345.local:9999", Yes: true, IgnoreConfig: true,
			NetworkId: s.controllerUnderTest.NetworkDialingIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.True(t,
			strings.Contains(err.Error(), hostErrors[0]) ||
				strings.Contains(err.Error(), hostErrors[1]) ||
				strings.Contains(err.Error(), hostErrors[2]) ||
				strings.Contains(err.Error(), "service 'non-existent-host-12345.local' not found"),
			"Error %s not contained in host errors array: %v", err.Error(), hostErrors)
		t.Logf("Invalid URL correctly failed: %v", err)
	})
}

func (s *cliTestState) testNonExistentUsername(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      "nonexistent-user-12345",
		Password:      s.controllerUnderTest.Password,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "non-existent username should fail")
	t.Logf("Non-existent username correctly failed: %v", err)
}

func (s *cliTestState) testControllerUnavailable(t *testing.T) {
	expectedErr := "connection refused"
	if runtime.GOOS == "windows" { //because of course it's different on linux/windows
		expectedErr = "the target machine actively refused it"
	}
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.controllerUnderTest.Username,
		Password:      s.controllerUnderTest.Password,
		ControllerUrl: "https://127.0.0.1:9999",
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "unavailable controller should fail")
	require.True(t,
		strings.Contains(err.Error(), expectedErr) ||
			strings.Contains(err.Error(), "service '127.0.0.1' not found"),
		"Expected error not found: %v", err.Error())

	t.Logf("Unavailable controller correctly failed: %v", err)
}

func (s *cliTestState) reconfigureTargetForZiti(pkiRoot string) error {
	v2 := cmd.NewRootCommand(os.Stdin, os.Stdout, os.Stderr)
	td := s.controllerUnderTest.TrustDomain
	iid := s.controllerUnderTest.InstanceID
	v2.SetArgs(strings.Split("pki create server --key-file server --pki-root "+pkiRoot+" --ip 127.0.0.1,::1 --dns localhost,mgmt,mgmt.ziti,mgmt-addressable-terminators --ca-name intermediate-ca-"+td+" --server-file mgmt.ziti --spiffe-id spiffe://"+td+"/controller/"+iid, " "))
	if zitiCmdErr := v2.Execute(); zitiCmdErr != nil {
		return zitiCmdErr
	}
	return nil
}

func (s *cliTestState) cliTestsOverZiti(t *testing.T, zitiPath string) {
	t.Run("cli tests over ziti", func(t *testing.T) {
		util.ReloadConfig() //every iteration needs to call reload to flush/overwrite the cached client in global state
		cfgDir := filepath.Join(s.homeDir, ".config/overlay")
		_ = os.Setenv("ZITI_CONFIG_DIR", cfgDir)
		_ = os.RemoveAll(cfgDir)
		s.controllerUnderTest.ControllerAddress = "mgmt.ziti"
		s.controllerUnderTest.ControllerPort = 443
		s.updateAdminIdFileForZiti(t, s.controllerUnderTest.ControllerHostPort())
		s.cliTests(t)
		s.testZitiThenNot(t)
	})
}

func (s *cliTestState) cliTestsOverAddressableTerminators(t *testing.T, zitiPath string) {
	t.Run("cli tests over ziti with addressable terminator", func(t *testing.T) {
		util.ReloadConfig() //every iteration needs to call reload to flush/overwrite the cached client in global state
		cfgDir := filepath.Join(s.homeDir, ".config/overlay-addressable-terminator")
		_ = os.Setenv("ZITI_CONFIG_DIR", cfgDir)
		_ = os.RemoveAll(cfgDir)
		s.controllerUnderTest.ControllerAddress = "mgmt-addressable-terminators"
		s.controllerUnderTest.ControllerPort = 443
		s.updateAdminIdFileForZiti(t, s.controllerUnderTest.ControllerHostPort())
		s.cliTests(t)
		s.testZitiThenNot(t)
	})
}
func (s *cliTestState) updateAdminIdFileForZiti(t *testing.T, newAddr string) {
	t.Logf("Updating %s with new url: %s from %s", s.controllerUnderTest.AdminIdFile, newAddr, s.controllerUnderTest.ControllerHostPort())

	data, _ := os.ReadFile(s.controllerUnderTest.AdminIdFile)

	re := regexp.MustCompile(`https://[^"]*/edge/client/v1`)
	out := re.ReplaceAllString(string(data), newAddr)

	_ = os.WriteFile(s.controllerUnderTest.AdminIdFile, []byte(out), 0644)
}

func (s *cliTestState) testSwitchController(t *testing.T) {
	// Test switching between two different controllers with different certs.
	// Ensures PopulateFromCache doesn't incorrectly inherit certs from previous controller.
	s.removeZitiDir(t)

	opts := s.controllerUnderTest.NewTestLoginOpts()
	opts.IgnoreConfig = false
	opts.Yes = true
	require.NoError(t, opts.Run(), "login to first controller should succeed")
	require.NotEmpty(t, opts.ApiSession)

	opts2 := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.externalZiti.Username,
		Password:      s.externalZiti.Password,
		ControllerUrl: s.externalZiti.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
	}
	require.NoError(t, opts2.Run(), "login to second controller should succeed")
	require.NotEmpty(t, opts2.ApiSession)

	opts3 := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.externalZiti.Username,
		Password:      s.externalZiti.Password,
		ControllerUrl: s.externalZiti.ControllerHostPort(),
		Yes:           false,
		IgnoreConfig:  false,
	}
	require.NoError(t, opts3.Run(), "re-login to second controller should use cached cert (-y now false)")
	require.NotEmpty(t, opts3.ApiSession)
}

func (s *cliTestState) testSameControllerUsesCache(t *testing.T) {
	// Regression test: re-logins to same controller should use cached cert without prompting.
	s.removeZitiDir(t)

	opts := s.controllerUnderTest.NewTestLoginOpts()
	opts.Yes = true
	require.NoError(t, opts.Run(), "first login should succeed")
	require.NotEmpty(t, opts.ApiSession)

	opts2 := s.controllerUnderTest.NewTestLoginOpts()
	opts2.Yes = false // errors if cert prompt fires
	require.NoError(t, opts2.Run(), "second login to same controller should use cached cert")
	require.NotEmpty(t, opts2.ApiSession)
}

func (s *cliTestState) testSwitchControllerFileAuth(t *testing.T) {
	// Test file-based auth to a different controller with cached certs from previous login.
	s.removeZitiDir(t)

	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.externalZiti.Username,
		Password:      s.externalZiti.Password,
		ControllerUrl: s.externalZiti.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
	}
	require.NoError(t, opts.Run(), "initial login should succeed")
	require.NotEmpty(t, opts.ApiSession)

	_, err := os.Stat(s.controllerUnderTest.AdminIdFile)
	require.NoError(t, err, "identity file must exist")

	opts2 := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		File:          s.controllerUnderTest.AdminIdFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}
	require.NotEmpty(t, opts2.File, "File must be set")

	te := opts2.Run()
	require.NoError(t, te, "file-based login to second controller should succeed")
	require.NotEmpty(t, opts2.ApiSession)
	require.NotNil(t, opts2.FileCertCreds)
}

func (s *cliTestState) testZitiThenNot(t *testing.T) {
	// Test switching from zitified login to non-zitified controller.
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.controllerUnderTest.Username,
		Password:      s.controllerUnderTest.Password,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.NoError(t, err, "overlay controller should login successfully")

	opts = &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.externalZiti.Username,
		Password:      s.externalZiti.Password,
		ControllerUrl: s.externalZiti.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		NetworkId:     "",
	}

	err = opts.Run()
	require.NoError(t, err, "underlay controller should login successfully")
}

func (s *cliTestState) testFileAuthIgnoresCachedServerCert(t *testing.T) {
	s.removeZitiDir(t)

	opts1 := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.externalZiti.Username,
		Password:      s.externalZiti.Password,
		ControllerUrl: s.externalZiti.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
	}
	require.NoError(t, opts1.Run(), "initial login to external controller should succeed")
	require.NotEmpty(t, opts1.ApiSession)

	// Find and corrupt the cached cert file for externalZiti
	certsDir := filepath.Join(s.homeDir, ".config", "underlay", "certs")
	require.DirExists(t, certsDir, "certs directory must exist after login")

	// List all cert files and find the one that was just created (most recent)
	entries, err := os.ReadDir(certsDir)
	require.NoError(t, err, "should be able to read certs directory")
	require.Greater(t, len(entries), 0, "at least one cert file should exist after login")

	// Find the most recently created cert file (should be from externalZiti login)
	var mostRecentCert string
	var mostRecentTime time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, _ := entry.Info()
		if info.ModTime().After(mostRecentTime) {
			mostRecentTime = info.ModTime()
			mostRecentCert = entry.Name()
		}
	}
	require.NotEmpty(t, mostRecentCert, "should find a recently created cert file")

	cachedCertPath := filepath.Join(certsDir, mostRecentCert)
	require.NoError(t, os.WriteFile(cachedCertPath, []byte("INVALID JUNK CERT DATA"), 0644), "corrupt cached cert file")

	_, err = os.Stat(s.controllerUnderTest.AdminIdFile)
	require.NoError(t, err, "identity file must exist")

	opts2 := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		File:          s.controllerUnderTest.AdminIdFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}
	require.NotEmpty(t, opts2.File, "File must be set")

	te := opts2.Run()
	require.NoError(t, te, "file-based login to different controller should succeed even with corrupted cached cert")
	require.NotEmpty(t, opts2.ApiSession)
	require.NotNil(t, opts2.FileCertCreds)

	// Verify that the cached cert remained corrupted (file-based auth didn't use or repair it)
	cachedCertData, err := os.ReadFile(cachedCertPath)
	require.NoError(t, err, "cached cert file should still exist")
	require.Equal(t, "INVALID JUNK CERT DATA", string(cachedCertData), "cached cert should remain corrupted after file-based auth succeeds")
}

func (s *cliTestState) testTrustedServerCertUsesSystemStore(t *testing.T) {
	if s.controllerUnderTest.NetworkDialingIdFile != "" {
		t.Skip("OS-trusted server cert path only applies to a direct controller connection, not over a ziti overlay")
	}

	s.removeZitiDir(t)

	opts1 := s.controllerUnderTest.NewTestLoginOpts()
	opts1.CaCert = ""
	opts1.Yes = true
	require.NoError(t, opts1.Run(), "untrusted-path login should succeed and cache the CA")
	require.NotEmpty(t, opts1.ApiSession)
	require.NotEmpty(t, opts1.CaCert, "untrusted path should cache and reference a CA file")

	cachedCert := opts1.CaCert
	_, statErr := os.Stat(cachedCert)
	require.NoError(t, statErr, "cached CA file should exist after the untrusted-path login")

	// simulate the OS trusting the server cert (the harness controller is self-signed)
	caPEM, err := os.ReadFile(s.controllerUnderTest.AdminCaFile)
	require.NoError(t, err, "read controller CA")
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(caPEM), "controller CA should parse")

	opts2 := s.controllerUnderTest.NewTestLoginOpts()
	opts2.SetSystemCertPool(pool)
	opts2.Yes = true
	opts2.IgnoreConfig = true
	require.NoError(t, opts2.Run(), "trusted-path login should succeed via system trust")
	require.NotEmpty(t, opts2.ApiSession)
	require.Empty(t, opts2.CaCert, "trusted path should clear CaCert so the identity uses system trust")

	_, statErr = os.Stat(cachedCert)
	require.True(t, os.IsNotExist(statErr), "trusted path should remove the stale cached CA at %s", cachedCert)

	// an explicit --ca that can't verify the server must fail despite OS trust. --ca is detected via
	// Cmd.Flags().Changed, so a real cobra flag is required; setting opts.CaCert alone lets the probe clear it.
	wrongCa := filepath.Join(t.TempDir(), "wrong-ca.pem")
	require.NoError(t, os.WriteFile(wrongCa, []byte("not a certificate"), 0600))

	caCmd := &cobra.Command{Use: "login"}
	caCmd.Flags().String("ca", "", "")
	caCmd.Flags().Bool("read-only", false, "")
	require.NoError(t, caCmd.Flags().Set("ca", wrongCa))

	opts3 := s.controllerUnderTest.NewTestLoginOpts()
	opts3.Cmd = caCmd
	opts3.CaCert = wrongCa
	opts3.Yes = true
	require.Error(t, opts3.Run(), "login must fail when --ca cannot validate the server, even if the OS trusts it")
}
