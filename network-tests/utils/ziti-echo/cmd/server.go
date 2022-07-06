package cmd

import (
	"net"
	"net/http"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
)

type zitiEchoServer struct {
	identityJson string
	listener     net.Listener
}

func (s *zitiEchoServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	input := r.URL.Query().Get("input")
	result := input
	//logrus.Infof("\nziti-http-echo-server: ")
	//logrus.Infof("received input '%v'\n", input)
	if _, err := rw.Write([]byte(result)); err != nil {
		panic(err)
	}
}

func (s *zitiEchoServer) run() (err error) {
	config, err := config.NewFromFile(s.identityJson)
	if err != nil {
		return err
	}

	zitiContext := ziti.NewContextWithConfig(config)
	if s.listener, err = zitiContext.Listen("echo"); err != nil {
		return err
	}

	//logrus.Info("\nziti-http-echo-server: ")
	//logrus.Info("listening for connections from echo server")
	go func() { _ = http.Serve(s.listener, http.HandlerFunc(s.ServeHTTP)) }()
	return nil
}

func (s *zitiEchoServer) stop() error {
	return s.listener.Close()
}

/*func main() {
	var identityJson string
	flag.StringVar(&identityJson, "identity", "", "The identity file to use")
	flag.Parse()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	echoServer := &zitiEchoServer{
		identityJson: identityJson,
	}
	if err := echoServer.run(); err != nil {
		log.Fatal(err)
	}

	for {
		select {
		//case <-time.After(10 * time.Second):
		//logrus.Debug("Beating to show server still running")
		case <-sigs:
			logrus.Info("Server shutting down...")
			return
		}
	}

}*/
