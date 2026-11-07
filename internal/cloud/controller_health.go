package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func free(port int) bool {
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// healthCheck bounds one ask of a server's health, so a server that takes the connection and never answers is asked again.
var healthCheck = &http.Client{Timeout: 2 * time.Second}

// answers waits for the organization's server to say it is well, for as long as c.Wait. A server whose process has ended will
// not, so it fails at once with the end of what it wrote. A node that cannot be asked says a server is not running too, so it
// takes endedAfter answers in a row to call it ended.
func (c *Controller) answers(ctx context.Context, node Node, spec ServerSpec, port int) error {
	ctx, cancel := context.WithTimeout(ctx, c.Wait)
	defer cancel()
	base := "http://" + net.JoinHostPort(node.Host(), strconv.Itoa(port))
	every := time.NewTicker(100 * time.Millisecond)
	defer every.Stop()
	for gone := 0; ; {
		if healthy(ctx, base+"/healthz") {
			return nil
		}
		if gone = gone + 1; node.Alive(spec.Slug) {
			gone = 0
		}
		if gone >= endedAfter {
			return fmt.Errorf("the server ended as it started: %s", lastWords(spec.Log))
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("the server did not answer in %s, and what it said is in server.log", c.Wait)
			}
			return ctx.Err()
		case <-every.C:
		}
	}
}

// endedAfter is how many answers in a row that a server is not running call it ended.
const endedAfter = 3

// lastWords is the end of a server's log, on one line, for the reason its start failed: at most the last 4 KB read, and the
// last 300 characters of that kept.
func lastWords(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return "it wrote no server.log"
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil && info.Size() > 4096 {
		file.Seek(-4096, io.SeekEnd)
	}
	tail, _ := io.ReadAll(io.LimitReader(file, 4096))
	said := strings.Join(strings.Fields(string(tail)), " ")
	if len(said) > 300 {
		said = "…" + said[len(said)-300:]
	}
	if said == "" {
		return "its server.log is empty"
	}
	return said + " (from server.log)"
}

// healthy is whether the server at address says it is well.
func healthy(ctx context.Context, address string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return false
	}
	response, err := healthCheck.Do(request)
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}
