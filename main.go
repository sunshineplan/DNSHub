package main

import (
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sunshineplan/service"
	"github.com/sunshineplan/utils/flags"
	"github.com/sunshineplan/utils/log"
)

var svc = service.New()

var (
	primary  = flag.String("primary", "", `List of primary DNS, separated with commas`)
	backup   = flag.String("backup", "", `List of backup DNS`)
	exclude  = flag.String("exclude", "", "Exclusion list `file` which only use backup DNS")
	hosts    = flag.String("hosts", "", "Hosts `file`")
	mode     = flag.String("mode", "UDP", "DNS mode (UDP, TCP, DoT, DoH)")
	port     = flag.Int("port", 0, "DNS server port (default: UDP&TCP-53, DoT-853, DoH-443)")
	cert     = flag.String("cert", "", "Path to certificate file, for DoT or DoH mode")
	privkey  = flag.String("privkey", "", "Path to private key file, for DoT or DoH mode")
	unix     = flag.String("unix", "", "Path to Unix socket, only for DoH mode")
	dnsProxy = flag.String("proxy", "", "List of proxies for DNS")
	fallback = flag.Bool("fallback", false, "Enable fallback")
	timeout  = flag.Duration("timeout", 5*time.Second, "Query timeout")
	logPath  = flag.String("log", "", "Path to log file")
	debug    = flag.Bool("debug", false, "debug")
)

func init() {
	svc.Name = "DNSHub"
	svc.Desc = "Instance to serve DNSHub"
	svc.Exec = run
	svc.TestExec = test
	svc.Options = service.Options{
		Dependencies: []string{"Wants=network-online.target", "After=network.target"},
	}
}

func main() {
	self, err := os.Executable()
	if err != nil {
		svc.Fatalln("Failed to get self path:", err)
	}
	flag.StringVar(&svc.Options.UpdateURL, "update", "", "Update URL")
	flags.SetConfigFile(filepath.Join(filepath.Dir(self), "config.ini"))
	flags.Parse()

	if *logPath != "" {
		svc.Logger = log.New(*logPath, "", log.LstdFlags)
	} else if !*debug {
		svc.Logger = log.New("", "", 0)
	}
	if *debug {
		svc.Logger.SetLevel(slog.LevelDebug)
	}
	if *exclude == "" {
		*exclude = filepath.Join(filepath.Dir(self), "exclude.list")
	}
	if *mode = strings.ToLower(*mode); *port == 0 {
		switch *mode {
		case "udp", "tcp":
			*port = 53
		case "dot":
			*port = 853
		case "doh":
			*port = 443
		default:
			svc.Fatalln("Invalid mode:", *mode)
		}
	}

	if err := svc.ParseAndRun(flag.Args()); err != nil {
		svc.Fatal(err)
	}
}
