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

package ops

import (
	"io"
	"os"

	"github.com/natefinch/lumberjack"
	"github.com/spf13/cobra"
)

type logPipeOptions struct {
	maxSizeMb  int
	maxBackups int
	maxAgeDays int
	compress   bool
}

// NewCmdLogPipe creates the "log-pipe" command. It copies stdin to a size-rotated
// file using the same rotation (lumberjack) as the controller's event and metrics
// logs. It's intended as a redirect target for long-running processes whose stdout
// would otherwise grow unbounded or be lost on restart, e.g.:
//
//	ziti controller run ... 2>&1 | ziti ops log-pipe ctrl.log --max-size-mb 50 --max-backups 10
func NewCmdLogPipe(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &logPipeOptions{}

	cmd := &cobra.Command{
		Use:   "log-pipe <file>",
		Short: "Copy stdin to a size-rotated log file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			writer := &lumberjack.Logger{
				Filename:   args[0],
				MaxSize:    options.maxSizeMb,
				MaxBackups: options.maxBackups,
				MaxAge:     options.maxAgeDays,
				Compress:   options.compress,
			}
			defer func() { _ = writer.Close() }()

			_, err := io.Copy(writer, os.Stdin)
			return err
		},
	}

	cmd.Flags().IntVar(&options.maxSizeMb, "max-size-mb", 50, "max size in MB before a file is rotated")
	cmd.Flags().IntVar(&options.maxBackups, "max-backups", 10, "max number of rotated files to keep (0 keeps all)")
	cmd.Flags().IntVar(&options.maxAgeDays, "max-age-days", 0, "max age in days to keep rotated files (0 = no age limit)")
	cmd.Flags().BoolVar(&options.compress, "compress", false, "gzip rotated files")

	return cmd
}
