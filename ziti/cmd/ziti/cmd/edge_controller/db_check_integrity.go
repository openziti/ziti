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
	edgeOptions
	fix bool
}

func newDbCheckIntegrityCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &dbCheckIntegrityOptions{
		edgeOptions: edgeOptions{
			CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "start-check-integrity",
		Short: "starts background operation checking integrity of database references and constraints",
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
	var target string
	if o.fix {
		target = "database/fix-data-integrity"
	} else {
		target = "database/check-data-integrity"
	}

	if _, err := util.EdgeControllerUpdate(target, "", o.Out, http.MethodPost, o.OutputJSONRequest, o.OutputJSONResponse, o.edgeOptions.Timeout, o.edgeOptions.Verbose); err != nil {
		return err
	}

	_, err := fmt.Fprint(o.Out, "check integrity operation started\n")
	return err
}

func newDbCheckIntegrityStatusCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &dbCheckIntegrityOptions{
		edgeOptions: edgeOptions{
			CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "check-integrity-status",
		Short: "shows current results from background operation checking integrity of database references and constraints",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCheckIntegrityStatus(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)

	return cmd
}

func runCheckIntegrityStatus(o *dbCheckIntegrityOptions) error {
	body, err := util.EdgeControllerList("database/data-integrity-results", nil, o.OutputJSONResponse, o.Out, o.edgeOptions.Timeout, o.edgeOptions.Verbose)
	if err != nil {
		return err
	}

	data := body.S("data")
	inProgress := data.S("inProgress").Data()
	if _, err = fmt.Fprintf(o.Out, "In Progress: %v\n", inProgress); err != nil {
		return err
	}

	fixingErrors := data.S("fixingErrors").Data()
	if _, err = fmt.Fprintf(o.Out, "Fixing Errors: %v\n", fixingErrors); err != nil {
		return err
	}

	tooManyErrors := data.S("tooManyErrors").Data()
	if _, err = fmt.Fprintf(o.Out, "Too Many Errors: %v (if true, additional errors can be found in controller log)\n", tooManyErrors); err != nil {
		return err
	}

	startTime := data.S("startTime").Data()
	if _, err = fmt.Fprintf(o.Out, "Started At: %v\n", startTime); err != nil {
		return err
	}

	endTime := data.S("endTime").Data()
	if _, err = fmt.Fprintf(o.Out, "Finished At: %v\n", endTime); err != nil {
		return err
	}

	opError := data.S("error").Data()
	if _, err = fmt.Fprintf(o.Out, "Operation Error: %v\n", opError); err != nil {
		return err
	}

	results := data.S("results")
	children, err := results.Children()
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
