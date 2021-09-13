package edge

import (
	"fmt"
	"github.com/fatih/color"
	"net"
	"net/http"
)

type plainEchoServer struct {
	Port     int
	listener net.Listener
}

func (s *plainEchoServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	input := r.URL.Query().Get("input")
	result := fmt.Sprintf("As you say, '%v', indeed!\n", input)
	c := color.New(color.FgBlue, color.Bold)
	c.Print("\nplain-http-echo-server: ")
	fmt.Printf("received input '%v'\n", input)
	if _, err := rw.Write([]byte(result)); err != nil {
		panic(err)
	}
}

func (s *plainEchoServer) run() (err error) {
	bindAddr := fmt.Sprintf("127.0.0.1:%v", s.Port)
	s.listener, err = net.Listen("tcp", bindAddr)
	if err != nil {
		return err
	}

	addr := s.listener.Addr().(*net.TCPAddr)
	s.Port = addr.Port

	c := color.New(color.FgBlue, color.Bold)
	c.Print("\nplain-http-echo-server: ")
	fmt.Printf("listening on %v\n", addr)
	go func() { _ = http.Serve(s.listener, http.HandlerFunc(s.ServeHTTP)) }()
	return nil
}

func (s *plainEchoServer) stop() error {
	return s.listener.Close()
}
