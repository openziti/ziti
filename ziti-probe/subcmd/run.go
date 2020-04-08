package subcmd

import (
	"context"
	influxdb "github.com/influxdata/influxdb1-client"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-foundation/metrics"
	"github.com/netfoundry/ziti-foundation/metrics/metrics_pb"
	"github.com/netfoundry/ziti-sdk-golang/ziti"
	"github.com/netfoundry/ziti-sdk-golang/ziti/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"sync/atomic"
	"time"
	"unsafe"
)

var runCmd = &cobra.Command{
	Use:   "run <config>",
	Short: "Runs ziti probe",
	Long:  "runs ziti probe: captures latency to edge routers and submits them to probe-service",
	Args:  cobra.MaximumNArgs(1),
	Run:   theProbe.run,
}

func init() {
	root.AddCommand(runCmd)
}

var log = pfxlog.Logger()

type probeCfg struct {
	dbName string
	dbType string
	interval int
}

var defaultConfig = probeCfg{
	dbName:   "ziti",
	dbType:   "influxdb",
	interval: 5 * 60, // 5 minutes
}

type probe struct{
	cfg *probeCfg
	indb *influxdb.Client
	ctx ziti.Context
	latest atomic.Value
	closer chan interface{}
}

var theProbe = &probe{
	cfg: &defaultConfig,
	closer: make(chan interface{}),
}

func (p *probe) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	p.latest.Store(message)
}

func (p *probe) sendMetrics() {
	for {
		select {
		case <-time.After(time.Duration(p.cfg.interval) * time.Second):
			message := p.latest.Load().(*metrics_pb.MetricsMessage)
			if message == nil {
				continue
			}
			bp, err := metrics.AsBatch(message)
			if err != nil {
				logrus.Errorln(err)
				return
			}
			bp.Database = p.cfg.dbName
			_, err = p.indb.Write(*bp)
			if err != nil {
				logrus.Errorln(err)
			}
		}
	}
}

func (p *probe) run(cmd *cobra.Command, args []string) {

	var err error
	if len(args) == 0 {
		log.Infof("loading ziti identity from env.ZITI_SDK_CONFIG[%s]", os.Getenv("ZITI_SDK_CONFIG"))
		p.ctx = ziti.NewContext()
	} else {
		if cfg, err := config.NewFromFile(args[0]); err != nil {
			log.Fatalf("failed to load config from file[%s]", args[0], err)
		} else {
			p.ctx = ziti.NewContextWithConfig(cfg)
		}

	}

	_, _ = p.ctx.GetServices()

	_, found := p.ctx.GetService("probe-service")
	if !found {
		log.Fatal("required service was not found")
	}

	p.indb, err = p.createInfluxDbClient()
	if err != nil {
		log.Error(err)
		return
	}
	_, res, err := p.indb.Ping()
	if err != nil {
		log.Fatalf("failed to get server info", err)
	}
	log.Info("connected to influx version = ", res)

	p.ctx.Metrics().EventController().AddHandler(p)

	go p.sendMetrics()

	select {
	case <-p.closer:
	}

    p.ctx.Close()
}

func (p *probe) createInfluxDbClient() (*influxdb.Client, error)  {

	httpC := &http.Client{
		Transport: &http.Transport{
			DialContext: func (c context.Context, network, addr string) (net.Conn, error) {
				log.Infof("connecting to %s:%s", network, addr)
				return p.ctx.Dial("probe-service")
			},
		},
	}

	config := influxdb.NewConfig()
	u, _ := url.Parse("http://probe-service")
	config.URL = *u
	clt, _ := influxdb.NewClient(config)

	ic := reflect.ValueOf(clt)
	ict := ic.Type()
	hcf, _ := ict.Elem().FieldByName("httpClient")

	hcp := unsafe.Pointer(uintptr(unsafe.Pointer(clt)) + hcf.Offset)

	hcpp := (**http.Client)(hcp)
	*hcpp = httpC

	return clt, nil
}
