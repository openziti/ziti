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
	"bytes"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/openziti/ziti/ziti/goprocess"
)

// PsOptions contains the command line options
type PsOptions struct {
	CommonOptions

	Flags PsFlags
}

type PsFlags struct {
	Pid string
}

var (
	psLong = templates.LongDesc(`
		Show information about currently running Ziti preocesses.
	`)
)

// NewCmdPs Pss a command object for the "Ps" command
func NewCmdPs(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PsOptions{
		CommonOptions: CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:   "ps",
		Short: "Show Ziti process info",
		Long:  psLong,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(NewCmdPsGet(out, errOut))
	cmd.AddCommand(NewCmdPsGoversion(out, errOut))
	cmd.AddCommand(NewCmdPsGc(out, errOut))
	cmd.AddCommand(NewCmdPsSetgc(out, errOut))
	cmd.AddCommand(NewCmdPsStack(out, errOut))
	cmd.AddCommand(NewCmdPsMemstats(out, errOut))
	cmd.AddCommand(NewCmdPsStats(out, errOut))
	cmd.AddCommand(NewCmdPsPprofHeap(out, errOut))
	cmd.AddCommand(NewCmdPsPprofCpu(out, errOut))
	cmd.AddCommand(NewCmdPsTrace(out, errOut))
	cmd.AddCommand(NewCmdPsSetLogLevel(out, errOut))
	cmd.AddCommand(NewCmdPsSetChannelLogLevel(out, errOut))
	cmd.AddCommand(NewCmdPsClearChannelLogLevel(out, errOut))

	return cmd
}

// Run implements this command
func (o *PsOptions) Run() error {
	fmt.Println("ps is running")
	ps := goprocess.FindAll()

	var maxPID, maxPPID, maxExec, maxVersion int
	for i, p := range ps {

		fmt.Println("p.Path is: " + p.Path)

		ps[i].BuildVersion = shortenVersion(p.BuildVersion)
		maxPID = max(maxPID, len(strconv.Itoa(p.PID)))
		maxPPID = max(maxPPID, len(strconv.Itoa(p.PPID)))
		maxExec = max(maxExec, len(p.Exec))
		maxVersion = max(maxVersion, len(ps[i].BuildVersion))

	}

	for _, p := range ps {
		buf := bytes.NewBuffer(nil)
		pid := strconv.Itoa(p.PID)
		fmt.Fprint(buf, pad(pid, maxPID))
		fmt.Fprint(buf, " ")

		ppid := strconv.Itoa(p.PPID)
		fmt.Fprint(buf, pad(ppid, maxPPID))
		fmt.Fprint(buf, " ")

		fmt.Fprint(buf, pad(p.Exec, maxExec))

		if p.Agent {
			fmt.Fprint(buf, "*")
		} else {
			fmt.Fprint(buf, " ")
		}
		fmt.Fprint(buf, " ")

		fmt.Fprint(buf, pad(p.BuildVersion, maxVersion))
		fmt.Fprint(buf, " ")

		fmt.Fprint(buf, p.Path)
		fmt.Fprintln(buf)
		buf.WriteTo(os.Stdout)
	}

	return nil
}

var develRe = regexp.MustCompile(`devel\s+\+\w+`)

func shortenVersion(v string) string {
	if !strings.HasPrefix(v, "devel") {
		return v
	}
	results := develRe.FindAllString(v, 1)
	if len(results) == 0 {
		return v
	}
	return results[0]
}

func pad(s string, total int) string {
	if len(s) >= total {
		return s
	}
	return s + strings.Repeat(" ", total-len(s))
}

func max(i, j int) int {
	if i > j {
		return i
	}
	return j
}
