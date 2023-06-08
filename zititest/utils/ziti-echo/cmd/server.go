package cmd

import (
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Simple Ziti enabled echo serer",
	Run:   runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	echoServer := &zitiEchoServer{
		identityJson: identityFile,
	}

	if err := echoServer.run(); err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case <-time.After(10 * time.Second):
			logrus.Debug("Beating to show server still running")
		case <-sigs:
			logrus.Info("Server shutting down...")
			return
		}
	}
}

type zitiEchoServer struct {
	identityJson string
	listener     net.Listener
}

func (s *zitiEchoServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	input := r.URL.Query().Get("input")
	result := input
	logrus.Infof("\nziti-http-echo-server: ")
	logrus.Infof("received input '%v'\n", input)
	if _, err := rw.Write([]byte(result)); err != nil {
		panic(err)
	}
}

func (s *zitiEchoServer) run() (err error) {
	zitiContext, err := ziti.NewContextFromFile(s.identityJson)
	if err != nil {
		return err
	}
	if s.listener, err = zitiContext.Listen("echo"); err != nil {
		return err
	}

	logrus.Info("\nziti-http-echo-server: ")
	logrus.Info("listening for connections from echo server")
	go func() { _ = http.Serve(s.listener, http.HandlerFunc(s.ServeHTTP)) }()
	return nil
}

func (s *zitiEchoServer) stop() error {
	return s.listener.Close()
}
