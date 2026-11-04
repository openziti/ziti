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

package agentcli

import (
	"context"
	"log/slog"
	"testing"

	"github.com/openziti/ziti/v2/common/logging"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// Test_agentCmd_Logging locks in that ziti agent keeps info logs off stderr unless --verbose is given.
// The channel library logs underlay changes at info through slog's default logger, and tooling such as
// the helm chart's readiness probe treats any stderr output from `ziti agent cluster list` as a failure.
func Test_agentCmd_Logging(t *testing.T) {
	t.Run("slog's default logger drops info and keeps warn", func(t *testing.T) {
		req := require.New(t)

		// arrange
		restoreLoggingAfter(t)
		agentCmd := NewAgentCmd(emptyOptions)

		// act
		agentCmd.PersistentPreRun(agentCmd, nil)

		// assert
		req.False(slog.Default().Enabled(context.Background(), slog.LevelInfo), "info logs from libraries using slog must not reach stderr")
		req.True(slog.Default().Enabled(context.Background(), slog.LevelWarn), "warnings must still reach stderr")
		req.Equal(logrus.WarnLevel, logrus.GetLevel())
	})

	t.Run("--verbose enables debug logging", func(t *testing.T) {
		req := require.New(t)

		// arrange
		restoreLoggingAfter(t)
		agentCmd := NewAgentCmd(emptyOptions)
		req.NoError(agentCmd.PersistentFlags().Set("verbose", "true"))

		// act
		agentCmd.PersistentPreRun(agentCmd, nil)

		// assert
		req.True(slog.Default().Enabled(context.Background(), slog.LevelDebug), "--verbose must enable debug logs from libraries using slog")
		req.Equal(logrus.DebugLevel, logrus.GetLevel())
	})
}

// emptyOptions provides the zero CommonOptions.
func emptyOptions() common.CommonOptions {
	return common.CommonOptions{}
}

// restoreLoggingAfter restores slog's default logger and the global log levels when the test ends.
func restoreLoggingAfter(t *testing.T) {
	prevDefault := slog.Default()
	prevLevel := logging.GlobalLevel()
	prevLogrusLevel := logrus.GetLevel()
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		logging.SetGlobalLevel(prevLevel)
		logrus.SetLevel(prevLogrusLevel)
	})
}
