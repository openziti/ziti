package smoke

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
)

var hashes = map[string]string{
	"1KB":   "0f343b0931126a20f133d67c2b018a3b",
	"100KB": "4c6426ac7ef186464ecbb0d81cbfcb1e",
	"20MB":  "8f4e33f3dc3e414ff94e5fb6905cba8c",
}

var timeouts = map[string]time.Duration{
	"1KB":   10 * time.Second,
	"100KB": 10 * time.Second,
	"20MB":  40 * time.Second,
}

type HttpClient string

const (
	ClientCurl HttpClient = "curl"
	ClientWget HttpClient = "wget"
)

var FileSizes = []string{"1KB", "100KB", "20MB"}
var HttpClients = []HttpClient{ClientCurl, ClientWget}

func TestFileDownload(hostSelector string, client HttpClient, hostType string, encrypted bool, fileSize string) (string, error) {
	host := model.GetModel().MustSelectHost("." + hostSelector + "-client")

	urlExtra := ""
	if !encrypted {
		urlExtra = "-unencrypted"
	}

	url := fmt.Sprintf("http://files-%s%s.ziti/%s.zip", hostType, urlExtra, fileSize)

	filename := uuid.NewString() + ".tmp"

	var cmds []string
	cmds = append(cmds, fmt.Sprintf("echo '%s  %s' > checksums", hashes[fileSize], filename))

	var cmd string
	if client == ClientCurl {
		cmd = fmt.Sprintf(`set -o pipefail; rm -f *.tmp; curl -k --fail-early --fail-with-body -SL -o %s %s 2>&1`, filename, url)
	} else if client == ClientWget {
		cmd = fmt.Sprintf(`set -o pipefail; rm -f *.tmp; wget --no-check-certificate -O %s -t 5 -T 5 %s 2>&1`, filename, url)
	}
	cmds = append(cmds, cmd)
	cmds = append(cmds, "md5sum -c checksums")

	timeout := timeouts[fileSize]
	return host.ExecLoggedWithTimeout(timeout, cmds...)
}

func TestIperf(clientHostSelector, hostType string, encrypted, reversed bool, run model.Run) (string, error) {
	c, err := model.GetModel().SelectComponent(".iperf." + hostType)
	if err != nil {
		return "", err
	}
	port := c.Type.(*zitilab.IPerfServerType).GetPort()
	// Stop and Start only issue their commands, so wait for the old server to release the port and
	// the new one to listen on it, or the client can reach neither
	if err = c.Type.Stop(run, c); err != nil {
		return "", err
	}
	if err = waitForListener(c, port, false, 10*time.Second); err != nil {
		return "", err
	}
	iperfServer := c.Type.(model.ServerComponent)
	if err = iperfServer.Start(run, c); err != nil {
		return "", err
	}
	if err = waitForListener(c, port, true, 10*time.Second); err != nil {
		return "", err
	}

	host, err := model.GetModel().SelectHost("." + clientHostSelector + "-client")
	if err != nil {
		return "", err
	}

	urlExtra := ""
	if !encrypted {
		urlExtra = "-unencrypted"
	}

	addr := fmt.Sprintf("iperf-%s%s.ziti", hostType, urlExtra)

	extraOptions := ""
	if reversed {
		extraOptions += " -R"
	}

	logFile := fmt.Sprintf("logs/iperf-%s%s%s.log", hostType, urlExtra, func() string {
		if reversed {
			return "-r"
		}
		return ""
	}())

	cmd := fmt.Sprintf(`set -o pipefail; iperf3 -c %s -P 5 -t 10 %s > %s 2>&1 `, addr, extraOptions, logFile)

	return host.ExecLoggedWithTimeout(40*time.Second, cmd)
}

// waitForListener waits up to timeout for a TCP listener on port of c's host, or with listening false,
// for there to be none. It fails if the state isn't reached in time.
func waitForListener(c *model.Component, port uint16, listening bool, timeout time.Duration) error {
	exitWhen := "&&"
	if !listening {
		exitWhen = "||"
	}
	cmd := fmt.Sprintf(`for i in $(seq 1 %d); do ss -Hltn "sport = :%d" | grep -q . %s exit 0; sleep 0.1; done; exit 1`,
		int(timeout/(100*time.Millisecond)), port, exitWhen)
	if _, err := c.GetHost().ExecLoggedWithTimeout(timeout+5*time.Second, cmd); err != nil {
		return fmt.Errorf("port %d on %s: listening never became %v (%w)", port, c.Id, listening, err)
	}
	return nil
}
