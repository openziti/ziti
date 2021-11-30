package logs

import (
	"github.com/spf13/cobra"
	"time"
)

func NewLogsCommand() *cobra.Command {
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "commands related to analyzing various ziti component logs",
	}

	parseRouterLogs := &ParseRouterLogs{}
	parseRouterLogs.Init()

	parseRouterLogsCmd := &cobra.Command{
		Use:   "router",
		Short: "Parse router logs",
		Args:  cobra.ExactArgs(1),
		RunE:  parseRouterLogs.run,
	}
	parseRouterLogsCmd.Flags().DurationVarP(&parseRouterLogs.bucketSize, "interval", "n", time.Hour, "Interval for which to aggregate log messages")

	logsCmd.AddCommand(parseRouterLogsCmd)

	return logsCmd
}

type ParseRouterLogs struct {
	JsonLogsParser
}

func (self *ParseRouterLogs) Init() {
	self.filters = append(self.filters,
		&filter{
			id:    "IDLE",
			label: "idle circuit scanner",
			desc:  "a circuit has been idle for at least one minute and the controller will be checked to see if the circuit is still valid",
			LogMatcher: AndMatchers(
				FieldContains("file", "forwarder/scanner.go"),
				FieldContains("msg", " idle after "),
			)},
		&filter{
			id:    "CONF",
			label: "circuit confirmation send",
			desc:  "controller has been notified of idle circuits and can respond if they are no longer valid",
			LogMatcher: AndMatchers(
				FieldContains("msg", "sent confirmation for "),
				FieldContains("file", "forwarder/scanner.go"),
			)},
		&filter{
			id:    "TSCX",
			label: "tunnel dial success",
			desc:  "a router embedded tunneler has made a successful connection to a server hosting an application",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "successful connection "),
				FieldContains("file", "xgress_edge_tunnel/dialer.go"),
			)},
		&filter{
			id:    "DSTX",
			label: "routing destination exists",
			desc:  "attempting to establish a circuit, but a previous dial attempt already established the egress connection",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "destination exists for "),
				FieldContains("file", "handler_ctrl/route.go"),
			)},
		&filter{
			id:    "XGRF",
			label: "xgress read failure",
			desc:  "read failure on the client or server side of the circuit; will cause the circuit to be torn down",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "read failed"),
				FieldContains("file", "xgress/xgress.go"),
			)},
		&filter{
			id:    "XGWF",
			label: "xgress write failure",
			desc:  "write failure on the client or server side of the circuit; will cause the circuit to be torn down",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "write failed"),
				FieldContains("file", "xgress/xgress.go"),
			)},
		&filter{
			id:    "XGFB",
			label: "xgress can't buffer",
			desc:  "while a circuit was being closed, data was received from the client or server",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "failure to buffer payload"),
				FieldEquals("error", "payload buffer closed"),
				FieldContains("file", "xgress/xgress.go"),
			)},
		&filter{
			id:    "CSTO",
			label: "circuit not started in time",
			desc:  "the terminating side of the circuit didn't receive a start from the initiating side",
			LogMatcher: AndMatchers(
				FieldEquals("msg", "xgress circuit not started in time, closing"),
				FieldContains("file", "xgress/xgress.go"),
			)},
		&filter{
			id:    "TLSC",
			label: "no tls certification provided",
			desc:  "a connection attempt was made to a TLS listener but no certificate was provided",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "error receiving hello from "),
				FieldContains("msg", "tls: client didn't provide a certificate"),
				FieldContains("file", "channel2/classic_listener.go"),
			)},
		&filter{
			id:    "LCYT",
			label: "latency timeout",
			desc:  "a latency ping was sent, but no response was received within the timeout",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "latency timeout after "),
				FieldContains("file", "metrics/latency.go"),
			)},
	)
}

func (self *ParseRouterLogs) run(_ *cobra.Command, args []string) error {
	ctx := &JsonParseContext{
		ParseContext: ParseContext{
			path: args[0],
		},
	}
	return ScanJsonLines(ctx, self.examineLogEntry)
}
