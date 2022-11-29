package edge

import (
	"fmt"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"math"
	"time"
)

type traceRouteOptions struct {
	api.Options
	skipIntermediate bool
	hops             uint8
	configFile       string
	timeout          time.Duration
	lookupRouterName bool
}

func newTraceRouteCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &traceRouteOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "traceroute <service> ",
		Short: "runs a traceroute on the service",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().StringVarP(&options.configFile, "config-file", "c", "", "Config file path")
	cmd.Flags().BoolVarP(&options.skipIntermediate, "skip-intermediate", "s", false, "Skip intermediate hops")
	cmd.Flags().Uint8Var(&options.hops, "hops", 0, "Maximum number of hops")
	cmd.Flags().DurationVarP(&options.timeout, "timeout", "t", 5*time.Second, "Trace route response timeout")
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
	cmd.Flags().BoolVarP(&options.lookupRouterName, "get-router-names", "n", false, "Lookup and output router names instead of ids. Requires admin privileges")

	return cmd
}

// Run implements this command
func (o *traceRouteOptions) Run() error {
	var ctx ziti.Context
	if o.configFile != "" {
		cfg, err := config.NewFromFile(o.configFile)
		if err != nil {
			return err
		}
		ctx = ziti.NewContextWithConfig(cfg)
	} else {
		ctx = ziti.NewContext()
	}

	conn, err := ctx.Dial(o.Args[0])
	if err != nil {
		return err
	}
	defer func() {
		if err = conn.Close(); err != nil {
			logrus.WithError(err).Error("failed to close connection")
		}
	}()

	hops := uint32(o.hops)
	if hops == 0 {
		hops = math.MaxUint32
	}
	currentHop := uint32(1)
	if o.skipIntermediate {
		currentHop = hops
	}

	routerNameLookupsFailed := false
	for currentHop <= hops {
		result, err := conn.TraceRoute(currentHop, o.timeout)
		if err != nil {
			return err
		}

		if result.Hops > 0 && result.Error == "" {
			break
		}

		hopLabel := result.HopId
		if result.HopType == "forwarder" {
			if o.lookupRouterName {
				hopLabel, err = mapIdToName("transit-routers", result.HopId, o.Options)
				if err != nil {
					hopLabel = result.HopId
					routerNameLookupsFailed = true
				}
			}
		}

		hopErr := ""
		if result.Error != "" {
			hopErr = fmt.Sprintf("ERROR=%v", result.Error)
		}

		if hopLabel == "" {
			fmt.Printf("%2v %25v %6v %v\n", currentHop, result.HopType, result.Time, hopErr)
		} else {
			fmt.Printf("%2v %25v %6v %v\n", currentHop, fmt.Sprintf("%v[%v]", result.HopType, hopLabel), result.Time, hopErr)
		}

		currentHop++
		if hopErr != "" {
			break
		}
	}

	if routerNameLookupsFailed {
		fmt.Println("Router name lookup failed. For this to work you must use ziti edge login first and be an administrator.")
	}
	return nil
}
