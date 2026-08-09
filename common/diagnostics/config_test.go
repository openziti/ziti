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

package diagnostics

import (
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func parse(t *testing.T, doc string) map[interface{}]interface{} {
	t.Helper()
	cfgmap := map[interface{}]interface{}{}
	require.NoError(t, yaml.Unmarshal([]byte(doc), &cfgmap))
	return cfgmap
}

func TestLoadConfig_AbsentSectionIsDisabledDefaults(t *testing.T) {
	req := require.New(t)

	for _, doc := range []string{"", "ctrl:\n  endpoint: tls:localhost:6262\n", "diagnostics: {}\n"} {
		cfg, err := LoadConfig(parse(t, doc))
		req.NoError(err)
		req.Equal(DefaultSlowHandlerConfig(), cfg, "a config that does not mention diagnostics must be the defaults")
		req.False(cfg.Enabled, "diagnostics must be off unless asked for")
	}
}

func TestLoadConfig_OverridesOnlyWhatIsNamed(t *testing.T) {
	req := require.New(t)

	cfg, err := LoadConfig(parse(t, `
diagnostics:
  slowHandlers:
    enabled: true
    stackDumpThreshold: 50ms
`))
	req.NoError(err)
	req.True(cfg.Enabled)
	req.Equal(50*time.Millisecond, cfg.StackDumpThreshold)
	req.Equal(DefaultSlowHandlerWarnThreshold, cfg.WarnThreshold, "an unset value keeps its default")
	req.Equal(DefaultSlowHandlerMaxSignatures, cfg.MaxSignatures)
	req.Equal(DefaultSlowHandlerDir, cfg.Dir)
}

func TestLoadConfig_DumpTrackingIsTunable(t *testing.T) {
	req := require.New(t)

	// What an investigation into something rare and specific would set: dump more often, keep more
	// distinct pictures, and treat smaller convoys as significant.
	cfg, err := LoadConfig(parse(t, `
diagnostics:
  slowHandlers:
    enabled: true
    stackDumpInterval: 30s
    maxStackDumps: 100
    minGoroutineGroup: 2
    maxStackDumpBytes: 33554432
    stackDumpDir: /tmp/dumps
`))
	req.NoError(err)
	req.Equal(30*time.Second, cfg.StackDumpInterval)
	req.Equal(100, cfg.MaxSignatures)
	req.Equal(2, cfg.MinGroup)
	req.Equal(33554432, cfg.MaxBytes)
	req.Equal("/tmp/dumps", cfg.Dir)
}

func TestLoadConfig_MaxBelowInitialDoesNotTruncateEveryDump(t *testing.T) {
	req := require.New(t)

	cfg, err := LoadConfig(parse(t, `
diagnostics:
  slowHandlers:
    enabled: true
    maxStackDumpBytes: 4096
`))
	req.NoError(err)
	req.Equal(4096, cfg.MaxBytes)
	req.Equal(4096, cfg.InitialBytes,
		"the buffer grows from initial to max, so a max below it would truncate every dump")
}

func TestLoadConfig_RejectsSettingsThatWouldProduceNothing(t *testing.T) {
	req := require.New(t)

	for name, doc := range map[string]string{
		"no dumps allowed": "diagnostics:\n  slowHandlers:\n    enabled: true\n    maxStackDumps: 0\n",
		// Zero never rate limits, and a snapshot is taken before it can be refused, so every slow
		// handler would snapshot every goroutine even once no dump can be written.
		"no rate limit":      "diagnostics:\n  slowHandlers:\n    enabled: true\n    stackDumpInterval: 0s\n",
		"negative interval":  "diagnostics:\n  slowHandlers:\n    enabled: true\n    stackDumpInterval: -1m\n",
		"zero threshold":     "diagnostics:\n  slowHandlers:\n    enabled: true\n    stackDumpThreshold: 0s\n",
		"zero warn":          "diagnostics:\n  slowHandlers:\n    enabled: true\n    warnThreshold: 0s\n",
		"zero group":         "diagnostics:\n  slowHandlers:\n    enabled: true\n    minGoroutineGroup: 0\n",
		"not a duration":     "diagnostics:\n  slowHandlers:\n    enabled: true\n    warnThreshold: 100\n",
		"not a bool":         "diagnostics:\n  slowHandlers:\n    enabled: yes-please\n",
		"not a whole number": "diagnostics:\n  slowHandlers:\n    enabled: true\n    maxStackDumps: lots\n",
	} {
		_, err := LoadConfig(parse(t, doc))
		req.Error(err, name)
	}
}

func TestLoadConfig_DisabledIsNotValidated(t *testing.T) {
	// A config left behind from an investigation should not stop a controller starting once it is
	// switched off, so the values are only checked when they will be used.
	_, err := LoadConfig(parse(t, "diagnostics:\n  slowHandlers:\n    enabled: false\n    maxStackDumps: 0\n"))
	require.NoError(t, err)
}

func TestNewSlowHandlerDetector_DisabledInstallsNothing(t *testing.T) {
	req := require.New(t)

	req.Nil(NewSlowHandlerDetector("ctrl", DefaultSlowHandlerConfig()), "a disabled config must build no detector")

	// A nil detector must be usable without a branch at the call site, and must not decorate the binding,
	// so a disabled configuration costs nothing per message rather than costing a check.
	var detector *SlowHandlerDetector
	inner := &countingBindHandler{}
	req.Same(inner, detector.Wrap(inner), "wrapping with no detector must return the handler itself")

	// Enabled, the binding the inner handler receives is the decorated one, which is what installs the
	// timing wrapper on each handler it registers.
	cfg := DefaultSlowHandlerConfig()
	cfg.Enabled = true
	enabled := NewSlowHandlerDetector("ctrl", cfg)
	req.NotNil(enabled)

	plain := &fakeBinding{}
	req.NoError(enabled.Wrap(inner).BindChannel(plain))
	req.NotNil(inner.got)
	req.NotSame(plain, inner.got, "an enabled detector must hand the inner handler a decorated binding")
	req.Equal(1, inner.bound)
}

type countingBindHandler struct {
	bound int
	got   channel.Binding
}

func (self *countingBindHandler) BindChannel(binding channel.Binding) error {
	self.bound++
	self.got = binding
	return nil
}

// fakeBinding stands in for a real binding. It embeds the interface, so anything this test does not
// exercise panics rather than silently doing nothing.
type fakeBinding struct{ channel.Binding }
