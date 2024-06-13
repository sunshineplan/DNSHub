package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/miekg/dns"
	"github.com/sunshineplan/utils/executor"
	"github.com/sunshineplan/utils/retry"
	"github.com/sunshineplan/utils/txt"
	"golang.org/x/net/proxy"
)

var (
	serverAddr                  string
	localDNSList, remoteDNSList []string

	trim = strings.TrimSpace
)

func formatDNSAddr(a string) string {
	hst, prt, err := net.SplitHostPort(a)
	if err != nil {
		hst = a
	}
	if trim(prt) == "" {
		prt = "53"
	}

	if ok, err := isLocal(trim(hst)); err != nil {
		svc.Print(err)
		return ""
	} else if ok && prt == strconv.Itoa(*port) {
		svc.Print("local dns or remote dns can not same as server dns address")
		return ""
	}

	return net.JoinHostPort(trim(hst), trim(prt))
}

func isLocal(host string) (bool, error) {
	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil {
			return false, err
		} else if len(ips) == 0 {
			return false, fmt.Errorf("lookup %s: no such host", host)
		}
		ip = ips[0]
	}

	if ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.Equal(net.ParseIP("::1")) {
		return true, nil
	}

	return false, nil
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func processDefault(w dns.ResponseWriter, r *dns.Msg) (err error) {
	resp, ok := getCache(r)
	if !ok {
		resp = r.Copy()
		q := resp.Question[0].Name
		qType := resp.Question[0].Qtype
		switch qType {
		case dns.TypeA, dns.TypeAAAA:
			ips, err := net.LookupIP(q)
			if err != nil {
				return err
			}

			for _, ip := range ips {
				var s string
				switch qType {
				case dns.TypeA:
					if ip.DefaultMask() != nil {
						s = fmt.Sprintf("%s A %s", q, ip)
					}
				case dns.TypeAAAA:
					if ip.DefaultMask() == nil {
						s = fmt.Sprintf("%s AAAA %s", q, ip)
					}
				}
				rr, err := dns.NewRR(s)
				if err != nil {
					svc.Println("failed to create record:", s)
					continue
				}
				if rr != nil {
					resp.Answer = append(resp.Answer, rr)
				}
			}
		case dns.TypeCNAME:
			cname, err := net.LookupCNAME(q)
			if err != nil {
				return err
			}
			s := fmt.Sprintf("%s CNAME %s", q, cname)
			rr, err := dns.NewRR(s)
			if err != nil {
				svc.Println("failed to create record:", s)
				break
			}
			resp.Answer = append(resp.Answer, rr)
		case dns.TypeTXT:
			txt, err := net.LookupTXT(q)
			if err != nil {
				return err
			}
			for _, i := range txt {
				s := fmt.Sprintf("%s TXT %q", q, i)
				rr, err := dns.NewRR(s)
				if err != nil {
					svc.Println("failed to create record:", s)
					continue
				}
				resp.Answer = append(resp.Answer, rr)
			}
		case dns.TypePTR:
			addr, err := net.LookupAddr(q)
			if err != nil {
				return err
			}
			for _, i := range addr {
				reverse, _ := dns.ReverseAddr(q)
				s := fmt.Sprintf("%s PTR %s", reverse, i)
				rr, err := dns.NewRR(s)
				if err != nil {
					svc.Println("failed to create record:", s)
					continue
				}
				resp.Answer = append(resp.Answer, rr)
			}
		case dns.TypeMX:
			mx, err := net.LookupMX(q)
			if err != nil {
				return err
			}
			for _, i := range mx {
				s := fmt.Sprintf("%s MX %d %s", q, i.Pref, i.Host)
				rr, err := dns.NewRR(s)
				if err != nil {
					svc.Println("failed to create record:", s)
					continue
				}
				resp.Answer = append(resp.Answer, rr)
			}
		case dns.TypeNS:
			ns, err := net.LookupNS(q)
			if err != nil {
				return err
			}
			for _, i := range ns {
				s := fmt.Sprintf("%s NS %s", q, i.Host)
				rr, err := dns.NewRR(s)
				if err != nil {
					svc.Println("failed to create record:", s)
					continue
				}
				resp.Answer = append(resp.Answer, rr)
			}
		case dns.TypeSRV:
			_, srv, err := net.LookupSRV("", "", q)
			if err != nil {
				return err
			}
			for _, i := range srv {
				s := fmt.Sprintf("%s SRV %d %d %d %s", q, i.Priority, i.Weight, i.Port, i.Target)
				rr, err := dns.NewRR(s)
				if err != nil {
					svc.Println("failed to create record:", s)
					continue
				}
				resp.Answer = append(resp.Answer, rr)
			}
		default:
			return fmt.Errorf("not supported query type for local lookup: %d", qType)
		}

		setCache(resp.Question, resp)
	} else {
		resp = resp.Copy()
		resp.Id = r.Id
	}

	return w.WriteMsg(resp)
}

func processLocal(w dns.ResponseWriter, r *dns.Msg, addr string) (err error) {
	resp, ok := getCache(r)
	if !ok {
		resp, err = dns.Exchange(r, addr)
		if err != nil {
			return
		}
		setCache(r.Question, resp)
	} else {
		resp = resp.Copy()
		resp.Id = r.Id
	}

	return w.WriteMsg(resp)
}

func processProxy(w dns.ResponseWriter, r *dns.Msg, p, addr string) error {
	resp, ok := getCache(r)
	if !ok {
		u, err := url.Parse(p)
		if err != nil || u.Host == "" {
			u, err = url.Parse("http://" + p)
			if err != nil {
				return err
			}
		}
		d, err := proxy.FromURL(u, nil)
		if err != nil {
			return err
		}
		conn, err := d.Dial("tcp", addr)
		if err != nil {
			return err
		}

		c := new(dns.Client)
		resp, _, err = c.ExchangeWithConn(r, &dns.Conn{Conn: conn})
		if err != nil {
			return err
		}
		setCache(r.Question, resp)
	} else {
		resp = resp.Copy()
		resp.Id = r.Id
	}

	return w.WriteMsg(resp)
}

func local(w dns.ResponseWriter, r *dns.Msg) error {
	if len(localDNSList) == 0 {
		return processDefault(w, r)
	} else {
		if _, err := executor.ExecuteConcurrentArg(
			localDNSList,
			func(addr string) (_ any, err error) { err = processLocal(w, r, addr); return },
			func(_ string) (_ any, err error) { err = processDefault(w, r); return },
		); err != nil {
			svc.Print(err)
			return err
		}
	}

	return nil
}

func remote(w dns.ResponseWriter, r *dns.Msg) (err error) {
	if len(remoteDNSList) == 0 {
		return errors.New("no remote dns provided")
	}

	if proxy := *dnsProxy; proxy != "" {
		_, err = executor.ExecuteConcurrentArg(
			remoteDNSList,
			func(addr string) (_ any, err error) { err = processProxy(w, r, proxy, addr); return },
		)
	} else {
		_, err = executor.ExecuteConcurrentArg(
			remoteDNSList,
			func(addr string) (_ any, err error) { err = processLocal(w, r, addr); return },
		)
	}
	if err != nil {
		svc.Print(err)
	}

	return
}

func loadDNSList() {
	*localDNS = trim(*localDNS)
	*remoteDNS = trim(*remoteDNS)

	if *localDNS == "" || *remoteDNS == "" {
		svc.Print("Only local dns or remote dns was provided, fallback will be enabled.")
		*fallback = true
	}

	for _, i := range splitComma(*localDNS) {
		if addr := formatDNSAddr(i); addr != "" {
			localDNSList = append(localDNSList, addr)
		}
	}

	for _, i := range splitComma(*remoteDNS) {
		if addr := formatDNSAddr(i); addr != "" {
			remoteDNSList = append(remoteDNSList, addr)
		}
	}
}

func registerHandler() {
	if *fallback {
		dns.DefaultServeMux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
			executor.ExecuteSerial(
				nil,
				func(_ any) (_ any, err error) { err = local(w, r); return },
				func(_ any) (_ any, err error) { err = remote(w, r); return },
			)
		})
		if len(remoteDNSList) != 0 {
			for _, i := range remoteList {
				dns.DefaultServeMux.HandleFunc(dns.Fqdn(i), func(w dns.ResponseWriter, r *dns.Msg) {
					executor.ExecuteSerial(
						nil,
						func(_ any) (_ any, err error) { err = remote(w, r); return },
						func(_ any) (_ any, err error) { err = local(w, r); return },
					)
				})
			}
		}
	} else {
		dns.DefaultServeMux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) { local(w, r) })
		if len(remoteDNSList) != 0 {
			for _, i := range remoteList {
				dns.DefaultServeMux.HandleFunc(dns.Fqdn(i), func(w dns.ResponseWriter, r *dns.Msg) { remote(w, r) })
			}
		}
	}
}

func reRegisterHandler() {
	for _, i := range remoteList {
		dns.DefaultServeMux.HandleRemove(dns.Fqdn(i))
	}
	var err error
	remoteList, err = txt.ReadFile(*list)
	if err != nil {
		svc.Print(err)
		return
	}
	if *fallback {
		if len(remoteDNSList) != 0 {
			for _, i := range remoteList {
				dns.DefaultServeMux.HandleFunc(dns.Fqdn(i), func(w dns.ResponseWriter, r *dns.Msg) {
					executor.ExecuteSerial(
						nil,
						func(_ any) (_ any, err error) { err = remote(w, r); return },
						func(_ any) (_ any, err error) { err = local(w, r); return },
					)
				})
			}
		}
	} else {
		if len(remoteDNSList) != 0 {
			for _, i := range remoteList {
				dns.DefaultServeMux.HandleFunc(dns.Fqdn(i), func(w dns.ResponseWriter, r *dns.Msg) { remote(w, r) })
			}
		}
	}
}

func run() (err error) {
	serverAddr, err = testDNSPort(*port)
	if err != nil {
		return fmt.Errorf("failed to test dns port: %v", err)
	}
	loadDNSList()
	parseHosts(*hosts)
	initRemoteList()

	svc.Printf("listen on: %s", serverAddr)
	return dns.ListenAndServe(serverAddr, "udp", dns.DefaultServeMux)
}

func test() error {
	*fallback = true
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

	loadDNSList()
	parseHosts(testHosts.Name())
	initRemoteList()
	go func() { ec <- dns.ListenAndServe(addr, "udp", dns.DefaultServeMux) }()

	var query = func(q, expected string) error {
		var r *dns.Msg
		m := new(dns.Msg).SetQuestion(q, dns.TypeA)
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
		if err := query("www.google.com.", ""); err != nil {
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
			svc.Print("test passed")
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
