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
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/openziti/ziti/ziti/cmd"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/util"
	"github.com/stretchr/testify/require"
)

func (s *cliTestState) loginTests(t *testing.T) {
	t.Run("correct password succeeds", s.testCorrectPasswordSucceeds)
	t.Run("wrong password fails", s.testWrongPasswordFails)
	t.Run("token based login", s.testTokenBasedLogin)
	t.Run("client cert authentication - no ca", s.testClientCertAuthentication)
	t.Run("identity file authentication", s.testIdentityFileAuthentication)
	t.Run("identity file authentication - ctrl url unset", s.testIdentityFileAuthenticationCtrlUrlUnset)
	t.Run("external JWT authentication", s.testExternalJWTAuthentication)
	t.Run("network identity zitified connection", s.testNetworkIdentityZitifiedConnection)

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
	v2.SetArgs(strings.Split("pki create server --key-file server --pki-root "+pkiRoot+" --ip 127.0.0.1,::1 --dns localhost,mgmt,mgmt.ziti,mgmt-addressable-terminators --ca-name intermediate-ca-quickstart --server-file mgmt.ziti", " "))
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

func (s *cliTestState) testZitiThenNot(t *testing.T) {
	// this test should make sure that after logging in with a zitified login, a subsequent login to a non-zitified
	// controller works as expected
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
