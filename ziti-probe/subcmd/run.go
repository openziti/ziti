//go:build all

package subcmd

import (
	"context"
	"fmt"
	influxdb "github.com/influxdata/influxdb1-client"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/metrics"
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
	ProbeService    = "probe-service"
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
	cfg      *probeCfg
	influxDb *influxdb.Client
	ctx      ziti.Context
	latest   atomic.Value
	closer   chan interface{}
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
			//TODO: currently broken as influx support was removed from metrics
			//      The AsBatch function can be extraced from foundation history
			//      if we want to get this working again
			bp, err := metrics.AsBatch(message)
			if err != nil {
				logrus.Errorln(err)
				return
			}
			bp.Database = p.cfg.dbName
			_, err = p.influxDb.Write(*bp)

			if err == nil {
				logrus.Debug("write complete")
			} else {
				logrus.Errorf("error writing to influxdb: %v", err)

				for {
					logrus.Info("attempting to reconnect")
					if service, ok := p.ctx.GetService(ProbeService); ok {
						if err := p.connectToInfluxDb(service); err == nil {
							logrus.Info("reconnected")
							if _, err = p.influxDb.Write(*bp); err != nil {
								logrus.Errorf("failed to write after reconnect")
							} else {
								break
							}
						} else {
							logrus.Errorf("failed to reconnect: %v", err)
						}
					} else {
						logrus.Errorf("could not find %s", ProbeService)
					}
					time.Sleep(30 * time.Second)
				}

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

	for {
		if err = p.ctx.Authenticate(); err != nil {
			sleepDuration := 5 * time.Second
			log.Errorf("failed to authenticate, trying again in %.2f seconds: %v", sleepDuration.Seconds(), err)
			time.Sleep(sleepDuration)
		} else {
			break
		}
	}

	for {
		if _, err = p.ctx.GetServices(); err != nil {
			sleepDuration := 5 * time.Second
			log.Errorf("failed to load available services, try again in %.2f seconds: %v", sleepDuration.Seconds(), err)
			time.Sleep(sleepDuration)
		} else {
			break
		}
	}

	var service *edge.Service
	for {
		var found bool
		service, found = p.ctx.GetService(ProbeService)
		if !found {
			sleepDuration := 5 * time.Second
			log.Errorf("required service was not found, try again in %.2f seconds", sleepDuration.Seconds())
			time.Sleep(sleepDuration)
		} else {
			break
		}
	}
	for {
		err := p.connectToInfluxDb(service)
		if err != nil {
			sleepDuration := 5 * time.Second
			log.Errorf("could not connect to influxDb, trying again in %.2f seconds: %v", sleepDuration.Seconds(), err)
			time.Sleep(sleepDuration)
		} else {
			break
		}
	}

	go p.sendMetrics()

	select {
	case <-p.closer:
	}

	p.ctx.Close()
}

func (p *probe) connectToInfluxDb(service *edge.Service) error {

	p.cfg = getProbeConfig(service)

	var err error
	p.influxDb, err = p.createInfluxDbClient(p.cfg)
	if err != nil {
		return fmt.Errorf("could not create influxDb client: %v", err)
	}

	_, res, err := p.influxDb.Ping()
	if err != nil {
		return fmt.Errorf("failed to get influxDb server info: %v", err)
	}
	log.Info("connected to influx version = ", res)
	return nil
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
	u, _ := url.Parse("http://" + ProbeService)
	influxConfig.URL = *u
	influxConfig.Username = cfg.dbUser
	influxConfig.Password = cfg.dbPassword
	clt, _ := influxdb.NewClient(influxConfig)

	ic := reflect.ValueOf(clt)
	ict := ic.Type()
	hcf, _ := ict.Elem().FieldByName("httpClient")

	hcp := unsafe.Pointer(uintptr(unsafe.Pointer(clt)) + hcf.Offset)

	httpCppClient := (**http.Client)(hcp)
	*httpCppClient = httpC

	return clt, nil
}
