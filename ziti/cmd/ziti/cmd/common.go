/*
	Copyright NetFoundry, Inc.

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

package cmd

import (
	"fmt"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/table"
	"github.com/openziti/ziti/ziti/cmd/ziti/internal/log"
	"github.com/spf13/cobra"
	"io"
	"os"
	"time"
)

// CommonOptions contains common options and helper methods
type CommonOptions struct {
	Factory        cmdutil.Factory
	Out            io.Writer
	Err            io.Writer
	Cmd            *cobra.Command
	Args           []string
	BatchMode      bool
	Verbose        bool
	Staging        bool
	ConfigIdentity string
	Timeout        int
}

type ServerFlags struct {
	ServerName string
	ServerURL  string
}

func (f *ServerFlags) IsEmpty() bool {
	return f.ServerName == "" && f.ServerURL == ""
}

func (c *CommonOptions) Stdout() io.Writer {
	if c.Out != nil {
		return c.Out
	}
	return os.Stdout
}

func (c *CommonOptions) CreateTable() table.Table {
	return c.Factory.CreateTable(c.Stdout())
}

// Debugf outputs the given text to the console if verbose mode is enabled
func (c *CommonOptions) Debugf(format string, a ...interface{}) {
	if c.Verbose {
		log.Infof(format, a...)
	}
}

func (options *CommonOptions) addCommonFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&options.BatchMode, "batch-mode", "b", false, "In batch mode the command never prompts for user input")
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
	cmd.Flags().BoolVarP(&options.Staging, "staging", "", false, "Install/Upgrade components from the ziti-staging repo")
	cmd.Flags().StringVarP(&options.ConfigIdentity, "configIdentity", "i", "", "Which configIdentity to use")
	cmd.Flags().IntVarP(&options.Timeout, "timeout", "t", 5, "Timeout for REST operations (specified in seconds)")
	options.Cmd = cmd
}

func (o *CommonOptions) retry(attempts int, sleep time.Duration, call func() error) (err error) {
	for i := 0; ; i++ {
		err = call()
		if err == nil {
			return
		}

		if i >= (attempts - 1) {
			break
		}

		time.Sleep(sleep)

		log.Infof("retrying after error:%s\n", err)
	}
	return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

func (o *CommonOptions) retryQuiet(attempts int, sleep time.Duration, call func() error) (err error) {
	lastMessage := ""
	dot := false

	for i := 0; ; i++ {
		err = call()
		if err == nil {
			if dot {
				log.Blank()
			}
			return
		}

		if i >= (attempts - 1) {
			break
		}

		time.Sleep(sleep)

		message := fmt.Sprintf("retrying after error: %s", err)
		if lastMessage == message {
			log.Info(".")
			dot = true
		} else {
			lastMessage = message
			if dot {
				dot = false
				log.Blank()
			}
			log.Infof("%s\n", lastMessage)
		}
	}
	return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

func (o *CommonOptions) retryQuietlyUntilTimeout(timeout time.Duration, sleep time.Duration, call func() error) (err error) {
	timeoutTime := time.Now().Add(timeout)

	lastMessage := ""
	dot := false

	for i := 0; ; i++ {
		err = call()
		if err == nil {
			if dot {
				log.Blank()
			}
			return
		}

		if time.Now().After(timeoutTime) {
			return fmt.Errorf("Timed out after %s, last error: %s", timeout.String(), err)
		}

		time.Sleep(sleep)

		message := fmt.Sprintf("retrying after error: %s", err)
		if lastMessage == message {
			log.Info(".")
			dot = true
		} else {
			lastMessage = message
			if dot {
				dot = false
				log.Blank()
			}
			log.Infof("%s\n", lastMessage)
		}
	}
}
