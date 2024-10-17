package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/miekg/dns"
	"github.com/sunshineplan/utils/retry"
)

func run() error {
	svc.Print("Start DNSHub")
	addr, err := testDNSPort(*port)
	if err != nil {
		return fmt.Errorf("failed to test dns port: %v", err)
	}

	svc.Debug("init proxy")
	initProxy()

	svc.Debug("init primary DNS")
	primary := parseClients(*primary)
	svc.Debug("init backup DNS")
	backup := parseClients(*backup)

	if *fallback {
		svc.Debug("allow fallback, add system DNS to backup")
		backup = append(backup, defaultResolver)
	}
	if len(backup) == 0 && len(primary) != 0 {
		svc.Debug("no backup DNS found, add system DNS to backup")
		backup = append(backup, defaultResolver)
	}
	if len(primary) == 0 {
		svc.Debug("no primary DNS found, add system DNS to primary")
		primary = append(primary, defaultResolver)
	}

	svc.Debug("init exclude list")
	exclude := initExcludeList(*exclude, backup)
	for _, i := range exclude {
		svc.Debug("exclude", "domain", i)
	}

	svc.Debug("init hosts")
	initHosts(*hosts)

	svc.Debug("init handle")
	initHandle(primary, backup)
	registerExclude(nil, exclude, backup)

	svc.Printf("listen on: %s", addr)
	return dns.ListenAndServe(addr, "udp", dns.DefaultServeMux)
}

func test() error {
	addr, err := testDNSPort(0)
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
		m := new(dns.Msg).SetQuestion(q, dns.TypeA)
		svc.Print(m.Question)
		return retry.Do(
			func() (err error) {
				r, err = dns.Exchange(m, addr)
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
			}, 5, 1,
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

func testDNSPort(port int) (string, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		return "", err
	}
	conn.Close()
	return conn.LocalAddr().String(), nil
}
