package edge_controller

import (
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
	"net/http"
)

type dbCheckIntegrityOptions struct {
	commonOptions
	fix bool
}

func newDbCheckIntegrityCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &dbCheckIntegrityOptions{
		commonOptions: commonOptions{
			CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "check-integrity",
		Short: "checks integrity of database references and constraints",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCheckIntegrityDb(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVarP(&options.fix, "fix-errors", "f", false, "Attempt to fix any detected errors")
	options.AddCommonFlags(cmd)

	return cmd
}

func runCheckIntegrityDb(o *dbCheckIntegrityOptions) error {
	var err error
	var body *gabs.Container

	if o.fix {
		body, err = util.EdgeControllerUpdate("database/fix-data-integrity", "", o.Out, http.MethodPost, o.OutputJSONRequest, o.OutputJSONResponse, o.commonOptions.Timeout, o.commonOptions.Verbose)
	} else {
		body, err = util.EdgeControllerList("database/check-data-integrity", nil, o.OutputJSONResponse, o.Out, o.commonOptions.Timeout, o.commonOptions.Verbose)
	}

	if err != nil {
		return err
	}

	data := body.S("data")
	children, err := data.Children()
	if len(children) == 0 || errors.Is(gabs.ErrNotObjOrArray, err) {
		_, err = fmt.Fprintln(o.Out, "no data integrity errors found")
	}

	for _, child := range children {
		desc := child.S("description").Data().(string)
		fixed := child.S("fixed").Data().(bool)

		if _, err = fmt.Fprintf(o.Out, "Issue: %v. Fixed: %v\n", desc, fixed); err != nil {
			return err
		}
	}

	return err
}
