package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"

	"github.com/brian-nunez/metricsd/internal/config"
	"github.com/brian-nunez/metricsd/internal/wire"
)

const usage = `Usage: metricsctl [-socket PATH] <command> [flags]

Commands:
  now                Show the latest value of every metric
  list               List available metrics
  history METRIC     Show averaged history for one metric
  health             Check that metricsd is running

Run 'metricsctl <command> -h' for command flags.
`

func main() {
	socket := flag.String("socket", config.DefaultSocketPath(), "path to the metricsd socket")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}

	c := newClient(*socket)
	cmd, args := flag.Arg(0), flag.Args()[1:]

	var err error
	switch cmd {
	case "now":
		err = cmdNow(c, args)
	case "list":
		err = cmdList(c, args)
	case "history":
		err = cmdHistory(c, args)
	case "health":
		err = cmdHealth(c, args)
	default:
		fmt.Fprintf(os.Stderr, "metricsctl: unknown command %q\n\n", cmd)
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "metricsctl:", err)
		os.Exit(1)
	}
}

type client struct {
	http   *http.Client
	socket string
}

func newClient(socket string) *client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
	}
	return &client{
		http:   &http.Client{Transport: transport, Timeout: 30 * time.Second},
		socket: socket,
	}
}

// get fetches path from metricsd and returns the raw JSON body.
func (c *client) get(path string, query url.Values) ([]byte, error) {
	u := "http://metricsd" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	resp, err := c.http.Get(u)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("cannot reach metricsd at %s (is it running?)", c.socket)
		}
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		var e wire.Error
		if json.Unmarshal(body, &e) == nil && e.Error != "" {
			return nil, errors.New(e.Error)
		}
		return nil, fmt.Errorf("metricsd returned %s", resp.Status)
	}
	return body, nil
}

func (c *client) getJSON(path string, query url.Values, out any) ([]byte, error) {
	body, err := c.get(path, query)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return body, nil
}

func printJSON(body []byte) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		return err
	}
	buf.WriteByte('\n')
	_, err := buf.WriteTo(os.Stdout)
	return err
}

// parseInterleaved parses flags that may appear before or after positional
// arguments, which the standard flag package does not support on its own.
func parseInterleaved(fs *flag.FlagSet, args []string) []string {
	var positional []string
	for {
		fs.Parse(args)
		if fs.NArg() == 0 {
			return positional
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}
