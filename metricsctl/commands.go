package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/brian-nunez/metricsd/internal/wire"
)

func cmdNow(c *client, args []string) error {
	fs := flag.NewFlagSet("now", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print raw JSON")
	if pos := parseInterleaved(fs, args); len(pos) > 0 {
		return fmt.Errorf("now: unexpected argument %q", pos[0])
	}

	var snap wire.Snapshot
	body, err := c.getJSON("/v1/metrics", nil, &snap)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(body)
	}
	if len(snap.Samples) == 0 {
		return errors.New("no samples collected yet")
	}

	slices.SortStableFunc(snap.Samples, func(a, b wire.Sample) int {
		if a.Metric != b.Metric {
			return naturalCompare(a.Metric, b.Metric)
		}
		return naturalCompare(a.Labels, b.Labels)
	})

	fmt.Printf("Collected %s\n\n", time.Unix(snap.TS, 0).Format(time.DateTime))
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "METRIC\tLABELS\tVALUE")
	for _, s := range snap.Samples {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Metric, s.Labels, formatValue(s.Metric, s.Value))
	}
	return tw.Flush()
}

func cmdList(c *client, args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print as a JSON array")
	if pos := parseInterleaved(fs, args); len(pos) > 0 {
		return fmt.Errorf("list: unexpected argument %q", pos[0])
	}

	var snap wire.Snapshot
	if _, err := c.getJSON("/v1/metrics", nil, &snap); err != nil {
		return err
	}

	counts := make(map[string]int)
	var names []string
	for _, s := range snap.Samples {
		if counts[s.Metric] == 0 {
			names = append(names, s.Metric)
		}
		counts[s.Metric]++
	}
	slices.SortFunc(names, naturalCompare)

	if *asJSON {
		return printJSONValue(names)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "METRIC\tSERIES")
	for _, n := range names {
		fmt.Fprintf(tw, "%s\t%d\n", n, counts[n])
	}
	return tw.Flush()
}

func cmdHistory(c *client, args []string) error {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	since := fs.String("since", "1h", "how far back to look (e.g. 30m, 6h, 7d)")
	step := fs.String("step", "", "bucket width for averaging (default: since/120)")
	labels := fs.String("labels", "", "only show the series with exactly these labels (e.g. core=all)")
	asJSON := fs.Bool("json", false, "print raw JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: metricsctl history METRIC [flags]")
		fs.PrintDefaults()
	}

	pos := parseInterleaved(fs, args)
	if len(pos) != 1 {
		fs.Usage()
		os.Exit(2)
	}

	q := url.Values{"since": {*since}}
	if *step != "" {
		q.Set("step", *step)
	}

	var hist wire.History
	body, err := c.getJSON("/v1/metrics/"+url.PathEscape(pos[0]), q, &hist)
	if err != nil {
		return err
	}

	if *labels != "" {
		hist.Series = slices.DeleteFunc(hist.Series, func(s wire.Series) bool { return s.Labels != *labels })
		if len(hist.Series) == 0 {
			return fmt.Errorf("no series of %s has labels %q", hist.Metric, *labels)
		}
	} else if *asJSON {
		return printJSON(body)
	}
	if *asJSON {
		return printJSONValue(hist)
	}

	slices.SortStableFunc(hist.Series, func(a, b wire.Series) int { return naturalCompare(a.Labels, b.Labels) })

	layout := time.TimeOnly
	if time.Since(time.Unix(hist.Since, 0)) > 24*time.Hour {
		layout = "Jan 02 15:04"
	}

	for i, s := range hist.Series {
		if i > 0 {
			fmt.Println()
		}
		name := hist.Metric
		if s.Labels != "" {
			name += "{" + s.Labels + "}"
		}
		fmt.Println(name)

		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
		for _, p := range s.Points {
			fmt.Fprintf(tw, "  %s\t%s\t\n", time.Unix(p.TS, 0).Format(layout), formatValue(hist.Metric, p.Value))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func cmdInfo(c *client, args []string) error {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print raw JSON")
	if pos := parseInterleaved(fs, args); len(pos) > 0 {
		return fmt.Errorf("info: unexpected argument %q", pos[0])
	}

	var info wire.Info
	body, err := c.getJSON("/v1/info", nil, &info)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(body)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(tw, "%s\t%s\n", k, v)
		}
	}
	row("Environment", info.Environment)
	row("ID", info.ID)
	row("Hostname", info.Hostname)
	row("OS", strings.TrimSpace(info.Platform+" "+info.PlatformVersion+" ("+info.OS+"/"+info.Arch+")"))
	row("Kernel", info.KernelVersion)
	row("CPU", info.CPUModel)
	row("Hypervisor", info.Hypervisor)
	if ct := info.Container; ct != nil {
		row("Container runtime", ct.Runtime)
		row("Container ID", ct.ID)
	}
	if k := info.Kubernetes; k != nil {
		row("Namespace", k.Namespace)
		row("Pod", k.Pod)
		row("Pod UID", k.PodUID)
		row("Container", k.Container)
		row("Node", k.Node)
	}
	keys := make([]string, 0, len(info.Tags))
	for k := range info.Tags {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		row("Tag "+k, info.Tags[k])
	}
	row("Agent version", info.AgentVersion)
	row("Started", time.Unix(info.StartedAt, 0).Format(time.DateTime))
	return tw.Flush()
}

func cmdHealth(c *client, args []string) error {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	if pos := parseInterleaved(fs, args); len(pos) > 0 {
		return fmt.Errorf("health: unexpected argument %q", pos[0])
	}

	var h wire.Health
	if _, err := c.getJSON("/v1/health", nil, &h); err != nil {
		return err
	}
	fmt.Println(h.Status)
	return nil
}
