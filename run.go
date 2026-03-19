package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnshttp"
	"github.com/sunshineplan/utils/httpsvr"
	"github.com/sunshineplan/utils/retry"
)

func run() (err error) {
	svc.Print("Start DNSHub")
	network := "tcp"
	if *mode == "udp" {
		network = "udp"
	}
	if *mode == "doh" && *unix != "" {
		*port = 0
	}
	var addr string
	if *port != 0 {
		addr, err = testDNSPort(network, *port)
		if err != nil {
			return fmt.Errorf("failed to test dns port: %w", err)
		}
	}

	if *debug {
		go http.ListenAndServe("localhost:6060", nil)
	}

	svc.Debug("init proxy")
	initProxy()

	var noPrimary, noBackup bool
	svc.Debug("init primary DNS")
	primary := parseClients(*primary)
	if len(primary) == 0 {
		noPrimary = true
	}
	svc.Debug("init backup DNS")
	backup := parseClients(*backup)
	if len(backup) == 0 {
		noBackup = true
	}

	if noPrimary && !*fallback {
		svc.Debug("no primary DNS found, add system DNS to primary")
		primary = append(primary, defaultResolver)
	}
	if noBackup && !*fallback {
		svc.Debug("no backup DNS found, add system DNS to backup")
		backup = append(backup, defaultResolver)
	}

	svc.Debug("init exclude list")
	exclude := initExcludeList(*exclude, primary, backup)
	for _, i := range exclude {
		svc.Debug("exclude", "domain", i)
	}

	svc.Debug("init hosts")
	initHosts(*hosts)

	svc.Debug("init handle")
	initHandle(primary, backup)
	registerExclude(nil, exclude, primary, backup)

	server := dns.NewServer()
	server.Addr = addr
	server.Net = network
	if *mode == "doh" {
		svr := httpsvr.New()
		svr.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m, err := dnshttp.Request(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			hw := dnshttp.NewResponseWriter(w, r, r.Context().Value(http.LocalAddrContextKey).(net.Addr))
			dns.DefaultServeMux.ServeDNS(r.Context(), hw, m)
		})
		if *unix != "" {
			svr.Unix = *unix
			return svr.Run()
		}
		if *cert != "" && *privkey != "" {
			svr.Port = strconv.Itoa(*port)
			svr.TLSConfig = &tls.Config{GetCertificate: getCertificate}
			return svr.Serve(true)
		}
		return errors.New("DoH mode needs Unix or Certificate to be set.")
	} else {
		svc.Printf("listen on: %s %s", network, addr)
		if *mode == "dot" {
			server.TLSConfig = &tls.Config{GetCertificate: getCertificate}
		}
		return server.ListenAndServe()
	}
}

func test() error {
	addr, err := testDNSPort("udp", 0)
	if err != nil {
		return fmt.Errorf("failed to get test address: %v", err)
	}

	testHosts, err := os.CreateTemp("", "")
	if err != nil {
		return err
	}
	testHosts.WriteString("  1.2.3.4\t \tdns.test.com\t \t\n")
	testHosts.Close()
	defer os.Remove(testHosts.Name())

	ec := make(chan error)
	rc := make(chan *dns.Msg)
	done := make(chan struct{})

	initHandle([]Client{defaultResolver}, nil)
	initHosts(testHosts.Name())
	go func() { ec <- dns.ListenAndServe(addr, "udp", dns.DefaultServeMux) }()

	var query = func(q, expected string) error {
		var r *dns.Msg
		m := dns.NewMsg(q, dns.TypeA)
		svc.Print(m.Question)
		return retry.Do(
			func() (err error) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				r, err = dns.Exchange(ctx, m, "udp", addr)
				if err != nil {
					return
				}
				if expected != "" {
					if result := fmt.Sprint(r.Answer); !strings.Contains(result, expected) {
						return fmt.Errorf("not expected result: %v", result)
					}
				}
				rc <- r
				return
			}, 5, 1*time.Second,
		)
	}
	go func() {
		if err := query("github.com.", ""); err != nil {
			ec <- err
		}
		if err := query("dns.test.com.", "1.2.3.4"); err != nil {
			ec <- err
		}
		done <- struct{}{}
	}()

	for {
		select {
		case err := <-ec:
			return err
		case msg := <-rc:
			if len(msg.Answer) == 0 {
				return errors.New("no result")
			}
			svc.Print(msg.Answer)
		case <-done:
			return nil
		}
	}
}

func testDNSPort(network string, port int) (string, error) {
	conn, err := net.Listen(network, fmt.Sprintf(":%d", port))
	if err != nil {
		return "", err
	}
	conn.Close()
	return conn.Addr().String(), nil
}
