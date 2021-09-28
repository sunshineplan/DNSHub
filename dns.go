package main

import (
	"errors"
	"log"
	"net"
	"path/filepath"
	"strings"

	"github.com/miekg/dns"
	"github.com/sunshineplan/utils"
	"github.com/sunshineplan/utils/executor"
	"github.com/sunshineplan/utils/txt"
	"golang.org/x/net/proxy"
)

func process(w dns.ResponseWriter, r *dns.Msg, s string) (err error) {
	resp, ok := getCache(r)
	if !ok {
		resp, err = dns.Exchange(r, s)
		if err != nil {
			return
		}
		setCache(r.Question, resp)
	}

	return w.WriteMsg(resp)
}

func processProxy(w dns.ResponseWriter, r *dns.Msg, p, s string) error {
	resp, ok := getCache(r)
	if !ok {
		d, err := proxy.SOCKS5("tcp", p, nil, nil)
		if err != nil {
			return err
		}
		conn, err := d.Dial("tcp", s)
		if err != nil {
			return err
		}

		c := new(dns.Client)
		resp, _, err = c.ExchangeWithConn(r, &dns.Conn{Conn: conn})
		if err != nil {
			return err
		}
		setCache(r.Question, resp)
	}

	return w.WriteMsg(resp)
}

func local(w dns.ResponseWriter, r *dns.Msg) error {
	if _, err := executor.ExecuteConcurrentArg(
		strings.Split(*localDNS, ","),
		func(i interface{}) (_ interface{}, err error) { err = process(w, r, i.(string)); return },
	); err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func remote(w dns.ResponseWriter, r *dns.Msg) (err error) {
	if proxy := *socks5Proxy; proxy != "" {
		_, err = executor.ExecuteConcurrentArg(
			strings.Split(*remoteDNS, ","),
			func(i interface{}) (_ interface{}, err error) { err = processProxy(w, r, proxy, i.(string)); return },
		)
	} else {
		_, err = executor.ExecuteConcurrentArg(
			strings.Split(*remoteDNS, ","),
			func(i interface{}) (_ interface{}, err error) { err = process(w, r, i.(string)); return },
		)
	}
	if err != nil {
		log.Print(err)
	}

	return
}

func registerHandler() {
	if *list == "" {
		*list = filepath.Join(filepath.Dir(self), "remotelist.txt")
	}
	remoteList, err := txt.ReadFile(*list)
	if err != nil {
		log.Print(err)
	}

	if *fallback {
		dns.DefaultServeMux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
			executor.ExecuteSerial(
				nil,
				func(_ interface{}) (_ interface{}, err error) { err = local(w, r); return },
				func(_ interface{}) (_ interface{}, err error) { err = remote(w, r); return },
			)
		})
		for _, i := range remoteList {
			dns.DefaultServeMux.HandleFunc(i, func(w dns.ResponseWriter, r *dns.Msg) {
				executor.ExecuteSerial(
					nil,
					func(_ interface{}) (_ interface{}, err error) { err = remote(w, r); return },
					func(_ interface{}) (_ interface{}, err error) { err = local(w, r); return },
				)
			})
		}
	} else {
		dns.DefaultServeMux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) { local(w, r) })
		for _, i := range remoteList {
			dns.DefaultServeMux.HandleFunc(i, func(w dns.ResponseWriter, r *dns.Msg) { remote(w, r) })
		}
	}
}

func run() {
	registerHandler()
	if err := dns.ListenAndServe(":53", "udp", dns.DefaultServeMux); err != nil {
		log.Fatal(err)
	}
}

func test() error {
	addr := getTestAddress()
	if addr == "" {
		return errors.New("failed to get test address")
	}

	ec := make(chan error)
	rc := make(chan *dns.Msg)
	registerHandler()
	go func() { ec <- dns.ListenAndServe(addr, "udp", dns.DefaultServeMux) }()
	go func() {
		var r *dns.Msg
		m := new(dns.Msg).SetQuestion("www.google.com.", dns.TypeA)
		if err := utils.Retry(
			func() (err error) {
				r, err = dns.Exchange(m, addr)
				if err != nil {
					return
				}
				rc <- r
				return
			}, 5, 1,
		); err != nil {
			ec <- err
		}
	}()

	select {
	case err := <-ec:
		return err
	case msg := <-rc:
		if len(msg.Answer) == 0 {
			return errors.New("no result")
		}
		log.Print(msg.Answer)
	}

	return nil
}

func getTestAddress() string {
	if conn, err := net.ListenUDP("udp", nil); err == nil {
		defer conn.Close()
		return conn.LocalAddr().String()
	}
	return ""
}
