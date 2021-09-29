package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/sunshineplan/service"
	_ "github.com/sunshineplan/utils/httpproxy"
	"github.com/vharitonsky/iniflags"
)

var (
	localDNS  = flag.String("local", "", "Local DNS")
	remoteDNS = flag.String("remote", "8.8.8.8", "Remote DNS")
	list      = flag.String("list", "", "Remote list `file`")
	hosts     = flag.String("hosts", "", "Hosts `file`")
	dnsProxy  = flag.String("proxy", "", "Remote DNS proxy")
	fallback  = flag.Bool("fallback", false, "Allow fallback")
)

var self string

var svc = service.Service{
	Name:     "Proxy DNS",
	Desc:     "Instance to serve Proxy DNS",
	Exec:     run,
	TestExec: test,
	Options: service.Options{
		Dependencies: []string{"After=network.target"},
	},
}

func init() {
	var err error
	self, err = os.Executable()
	if err != nil {
		log.Fatalln("Failed to get self path:", err)
	}
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `Usage of %s:
  -local <string>
    	List of local DNS servers, separated with commas. Port numbers may also optionally be
		given as :<port-number> after each address
  -remote <string>
    	List of remote DNS servers which must support tcp (default "8.8.8.8")
  -list <file>
    	Remote list file
  -hosts <file>
    	Hosts file
  -proxy <string>
    	Remote DNS proxy, support http,https,socks5,socks5h proxy
  -fallback
    	Enable fallback
  -update <url>
    	Update URL
%s`, os.Args[0], service.Usage)
}

func main() {
	flag.Usage = usage
	flag.StringVar(&svc.Options.UpdateURL, "update", "", "Update URL")
	iniflags.SetConfigFile(filepath.Join(filepath.Dir(self), "config.ini"))
	iniflags.SetAllowMissingConfigFile(true)
	iniflags.SetAllowUnknownFlags(true)
	iniflags.Parse()

	if service.IsWindowsService() {
		svc.Run(false)
		return
	}

	var err error
	switch flag.NArg() {
	case 0:
		run()
	case 1:
		switch flag.Arg(0) {
		case "run":
			svc.Run(false)
		case "debug":
			svc.Run(true)
		case "test":
			err = svc.Test()
		case "install":
			err = svc.Install()
		case "uninstall", "remove":
			err = svc.Uninstall()
		case "start":
			err = svc.Start()
		case "stop":
			err = svc.Stop()
		case "restart":
			err = svc.Restart()
		case "update":
			err = svc.Update()
		default:
			log.Fatalln(fmt.Sprintf("Unknown argument: %s", flag.Arg(0)))
		}
	default:
		log.Fatalln(fmt.Sprintf("Unknown arguments: %s", strings.Join(flag.Args(), " ")))
	}
	if err != nil {
		log.Fatalf("Failed to %s: %v", flag.Arg(0), err)
	}
}
