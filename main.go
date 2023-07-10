package main

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/sunshineplan/service"
	"github.com/sunshineplan/utils/flags"
	_ "github.com/sunshineplan/utils/httpproxy"
)

var (
	self string

	svc = service.New()
)

var (
	localDNS  = flag.String("local", "", `List of local DNS servers, separated with commas. Port numbers may also optionally be given as :<port-number> after each address`)
	remoteDNS = flag.String("remote", "8.8.8.8", `List of remote DNS servers which must support tcp (default "8.8.8.8")`)
	list      = flag.String("list", "", "Remote list `file`")
	hosts     = flag.String("hosts", "", "Hosts `file`")
	dnsProxy  = flag.String("proxy", "", "Remote DNS proxy, support http,https,socks5,socks5h proxy")
	port      = flag.Int("port", 53, "DNS port (default 53)")
	fallback  = flag.Bool("fallback", false, "Enable fallback")
)

func init() {
	var err error
	self, err = os.Executable()
	if err != nil {
		svc.Fatalln("Failed to get self path:", err)
	}
	svc.Name = "ProxyDNS"
	svc.Desc = "Instance to serve Proxy DNS"
	svc.Exec = run
	svc.TestExec = test
	svc.Options = service.Options{
		Dependencies: []string{"Wants=network-online.target", "After=network.target"},
	}
}

func main() {
	flag.StringVar(&svc.Options.UpdateURL, "update", "", "Update URL")
	flags.SetConfigFile(filepath.Join(filepath.Dir(self), "config.ini"))
	flags.Parse()

	if err := svc.ParseAndRun(flag.Args()); err != nil {
		svc.Fatal(err)
	}
}
