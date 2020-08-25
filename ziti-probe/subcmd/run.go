package subcmd

import (
	"context"
	influxdb "github.com/influxdata/influxdb1-client"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/openziti/sdk-golang/ziti/edge"
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

const (
	DefaultDbName   = "ziti"
	DefaultDbType   = "influxdb"
	DefaultInterval = 5 * 60 // 5 minutes
)

func init() {
	root.AddCommand(runCmd)
}

var log = pfxlog.Logger()

type probeCfg struct {
	dbName     string
	dbType     string
	dbUser     string
	dbPassword string
	interval   int
}

var defaultConfig = probeCfg{
	dbName:   DefaultDbName,
	dbType:   DefaultDbType,
	interval: DefaultInterval,
}

type probe struct {
	cfg    *probeCfg
	indb   *influxdb.Client
	ctx    ziti.Context
	latest atomic.Value
	closer chan interface{}
}

var theProbe = &probe{
	closer: make(chan interface{}),
}

func (p *probe) sendMetrics() {
	for {
		select {
		case <-time.After(time.Duration(p.cfg.interval) * time.Second):
			message := p.ctx.Metrics().Poll()
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

func (p *probe) run(_ *cobra.Command, args []string) {

	var err error
	if len(args) == 0 {
		log.Infof("loading ziti identity from env.ZITI_SDK_CONFIG[%s]", os.Getenv("ZITI_SDK_CONFIG"))
		p.ctx = ziti.NewContext()
	} else {
		if cfg, err := config.NewFromFile(args[0]); err != nil {
			log.WithError(err).Fatalf("failed to load config from file[%s]", args[0])
		} else {
			cfg.ConfigTypes = append(cfg.ConfigTypes, "ziti-probe-config.v1")
			p.ctx = ziti.NewContextWithConfig(cfg)
		}

	}

	if _, err = p.ctx.GetServices(); err != nil {
		log.Fatal("failed to load available services")
	}

	service, found := p.ctx.GetService("probe-service")
	if !found {
		log.Fatal("required service was not found")
	}

	p.cfg = getProbeConfig(service)

	p.indb, err = p.createInfluxDbClient(p.cfg)
	if err != nil {
		log.Error(err)
		return
	}

	_, res, err := p.indb.Ping()
	if err != nil {
		log.WithError(err).Fatal("failed to get server info")
	}
	log.Info("connected to influx version = ", res)

	go p.sendMetrics()

	select {
	case <-p.closer:
	}

	p.ctx.Close()
}

func getProbeConfig(service *edge.Service) *probeCfg {
	cfg, found := service.Configs["ziti-probe-config.v1"]
	if !found {
		return &defaultConfig
	}

	dbName := DefaultDbName
	if val, found := cfg["dbName"]; found {
		if str, ok := val.(string); ok {
			dbName = str
		}
	}

	dbType := DefaultDbType
	if val, found := cfg["dbType"]; found {
		if str, ok := val.(string); ok {
			dbType = str
		}
	}

	var dbUser string
	if val, found := cfg["dbUser"]; found {
		if str, ok := val.(string); ok {
			dbUser = str
		}
	}

	var dbPassword string
	if val, found := cfg["dbPassword"]; found {
		if str, ok := val.(string); ok {
			dbPassword = str
		}
	}

	interval := DefaultInterval
	if val, found := cfg["interval"]; found {
		if flt, ok := val.(float64); ok {
			interval = int(flt)
		}
	}

	return &probeCfg{
		dbName:     dbName,
		dbType:     dbType,
		dbUser:     dbUser,
		dbPassword: dbPassword,
		interval:   interval,
	}
}

func (p *probe) createInfluxDbClient(cfg *probeCfg) (*influxdb.Client, error) {

	httpC := &http.Client{
		Transport: &http.Transport{
			DialContext: func(c context.Context, network, addr string) (net.Conn, error) {
				log.Infof("connecting to %s:%s", network, addr)
				return p.ctx.Dial("probe-service")
			},
		},
	}

	influxConfig := influxdb.NewConfig()
	u, _ := url.Parse("http://probe-service")
	influxConfig.URL = *u
	influxConfig.Username = cfg.dbUser
	influxConfig.Password = cfg.dbPassword
	clt, _ := influxdb.NewClient(influxConfig)

	ic := reflect.ValueOf(clt)
	ict := ic.Type()
	hcf, _ := ict.Elem().FieldByName("httpClient")

	hcp := unsafe.Pointer(uintptr(unsafe.Pointer(clt)) + hcf.Offset)

	hcpp := (**http.Client)(hcp)
	*hcpp = httpC

	return clt, nil
}
