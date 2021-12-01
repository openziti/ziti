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
	parseRouterLogsCmd.Flags().IntVarP(&parseRouterLogs.maxUnmatchedLoggedPerBucket, "max-unmatched", "u", 1, "Maximum unmatched log messages to output per bucket")
	parseRouterLogsCmd.Flags().StringSliceVar(&parseRouterLogs.ignore, "ignore", []string{"IDLE", "CONF"}, "Filters to ignore")

	showRouterLogCategoriesCmd := &cobra.Command{
		Use:   "categories",
		Short: "Show router log entry categories",
		Run:   parseRouterLogs.ShowCategories,
	}

	parseRouterLogsCmd.AddCommand(showRouterLogCategoriesCmd)

	logsCmd.AddCommand(parseRouterLogsCmd)

	return logsCmd
}

type ParseRouterLogs struct {
	JsonLogsParser
}

func (self *ParseRouterLogs) Init() {
	self.filters = getRouterLogFilters()
}

func getRouterLogFilters() []LogFilter {
	return []LogFilter{
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
			id:    "XGBF",
			label: "xgress can't buffer",
			desc:  "while a circuit was being closed, data was received from the client or server",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "failure to buffer payload"),
				FieldEquals("error", "payload buffer closed"),
				FieldContains("file", "xgress/xgress.go"),
			)},
		&filter{
			id:    "RCFM",
			label: "can't forward",
			desc:  "router can't forward a message most likely because the circuit is in the middle of being torn down",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "unable to forward"),
				FieldContains("file", "handler_xgress/receive.go"),
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
		&filter{
			id:    "CHRX",
			label: "channel rx error",
			desc:  "channel read failed",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "rx error"),
				FieldContains("msg", "connection reset by peer"),
				FieldContains("file", "channel2/impl.go"),
			)},
		&filter{
			id:    "LNKS",
			label: "dialing split channel link",
			desc:  "a link is being dialed to another router with separate connections for data and acknowledgements",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "dialing link with split payload/ack channels"),
				FieldContains("file", "xlink_transport/dialer.go"),
			)},
		&filter{
			id:    "LNKR",
			label: "controller requesting dial",
			desc:  "received a request from the controller to dial another router",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "received link connect request"),
				FieldContains("file", "handler_ctrl/dial.go"),
			)},
		&filter{
			id:    "LNKD",
			label: "dialing link",
			desc:  "dialing another router to establish a link",
			LogMatcher: AndMatchers(
				FieldEquals("msg", "dialing link"),
				FieldContains("file", "handler_ctrl/dial.go"),
			)},
		&filter{
			id:    "LNKE",
			label: "link established",
			desc:  "a link to another router has been established",
			LogMatcher: AndMatchers(
				FieldEquals("msg", "link established"),
				FieldContains("file", "handler_ctrl/dial.go"),
			)},
		&filter{
			id:    "LNKP",
			label: "dialing link payload channel",
			desc:  "dialing the link payload channel of a link",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "dialing payload channel for"),
				FieldContains("file", "xlink_transport/dialer.go"),
			)},
		&filter{
			id:    "LNKK",
			label: "dialing link ack channel",
			desc:  "dialing the link ack channel of a link",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "dialing ack channel for"),
				FieldContains("file", "xlink_transport/dialer.go"),
			)},
		&filter{
			id:    "LNKC",
			label: "link closed",
			desc:  "a router to router link was closed",
			LogMatcher: AndMatchers(
				FieldEquals("msg", "link closed"),
				FieldContains("file", "handler_link/close.go"),
			)},
		&filter{
			id:    "FLTS",
			label: "link fault sent",
			desc:  "the router notified the controller that a link failed",
			LogMatcher: AndMatchers(
				FieldEquals("msg", "transmitted link fault"),
				FieldContains("file", "handler_link/close.go"),
			)},
		&filter{
			id:    "LNKA",
			label: "link accepted",
			desc:  "another router has dialed this router and a link has been established",
			LogMatcher: AndMatchers(
				FieldStartsWith("msg", "accepted new link"),
				FieldContains("file", "router/accepter.go"),
			)},
		&filter{
			id:    "RCNB",
			label: "starting reconnect",
			desc:  "the router to controller control channel connection died and the router trying to reconnect",
			LogMatcher: AndMatchers(
				FieldContains("msg", "starting reconnection process"),
				FieldContains("file", "channel2/reconnecting_impl.go"),
			)},
		&filter{
			id:    "RCNF",
			label: "reconnect failed",
			desc:  "the router attempted to reconnect the control channel and failed",
			LogMatcher: AndMatchers(
				FieldContains("file", "channel2/reconnecting_dialer.go"),
				FieldMatches("msg", "reconnection attempt.*failed"),
			)},
		&filter{
			id:    "RCNS",
			label: "reconnect succeeded",
			desc:  "the router attempted to reconnect the control channel and succeeded",
			LogMatcher: AndMatchers(
				FieldEquals("msg", "reconnected"),
				FieldContains("file", "channel2/reconnecting_impl.go"),
			)},
		&filter{
			id:    "RCNP",
			label: "reconnect ping",
			desc:  "the router is checking the control channel to see if it needs to be reconnected",
			LogMatcher: AndMatchers(
				FieldContains("file", "channel2/reconnecting_impl.go"),
				FieldContains("func", "pingInstance"),
			)},
		&filter{
			id:    "RCPF",
			label: "reconnect ping failed",
			desc:  "the router control channel ping failed",
			LogMatcher: AndMatchers(
				FieldContains("msg", "unable to ping"),
				FieldContains("file", "channel2/reconnecting_dialer.go"),
			)},
		&filter{
			id:    "HELO",
			label: "received ctrl hello",
			desc:  "the controller sent us a hello message after a control channel was established or reconnected",
			LogMatcher: AndMatchers(
				FieldContains("msg", "received server hello"),
				FieldContains("file", "handler_edge_ctrl/hello.go"),
			)},
		&filter{
			id:    "SSST",
			label: "api session sync started",
			desc:  "The controller has started a full api session sync",
			LogMatcher: AndMatchers(
				FieldContains("file", "handler_edge_ctrl/apiSessionAdded.go"),
				FieldMatches("msg", "api session.*starting"),
			)},
		&filter{
			id:    "SSCH",
			label: "api session data received",
			desc:  "The controller sent a chunk of api session data",
			LogMatcher: AndMatchers(
				FieldContains("file", "handler_edge_ctrl/apiSessionAdded.go"),
				FieldStartsWith("msg", "received api session sync chunk"),
			)},
		&filter{
			id:    "SSDN",
			label: "api session sync finished",
			desc:  "The controller has finished a full api session sync",
			LogMatcher: AndMatchers(
				FieldContains("file", "handler_edge_ctrl/apiSessionAdded.go"),
				FieldStartsWith("msg", "finished sychronizing api sessions"),
			)},
		&filter{
			id:    "CHAC",
			label: "accepted connection",
			desc:  "a tcp connection has been accepted",
			LogMatcher: AndMatchers(
				FieldEquals("msg", "accepted connection"),
				FieldContains("file", "transport/tcp/listener.go"),
			)},
	}
}

func (self *ParseRouterLogs) run(cmd *cobra.Command, args []string) error {
	if err := self.validate(); err != nil {
		return err
	}

	ctx := &JsonParseContext{
		ParseContext: ParseContext{
			path: args[0],
		},
	}
	return ScanJsonLines(ctx, self.examineLogEntry)
}
