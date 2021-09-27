package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/sunshineplan/utils/cache"
	"github.com/sunshineplan/utils/executor"
	"github.com/sunshineplan/utils/txt"
	"golang.org/x/net/proxy"
)

var (
	localDNS    = flag.String("local", "127.0.0.1:53", "local dns")
	remoteDNS   = flag.String("remote", "8.8.8.8:53", "remote dns")
	list        = flag.String("list", "remotelist.txt", "remote list file")
	socks5Proxy = flag.String("socks5", "127.0.0.1:8888", "socks5 proxy")
	fallback    = flag.Bool("fallback", false, "Allow fallback")
)

var dnsCache = cache.New(true)

func getCache(r *dns.Msg) (*dns.Msg, bool) {
	value, ok := dnsCache.Get(fmt.Sprint(r.Question))
	if ok {
		v := value.(*dns.Msg)
		v.Id = r.Id
		return v, true
	}
	return nil, false
}

func setCache(key interface{}, r *dns.Msg) {
	if len(r.Answer) == 0 {
		dnsCache.Set(fmt.Sprint(key), r, 300*time.Second, nil)
		return
	}
	dnsCache.Set(fmt.Sprint(key), r, time.Duration(r.Answer[0].Header().Ttl)*time.Second, nil)
}

func process(w dns.ResponseWriter, r *dns.Msg, s string) error {
	var resp *dns.Msg
	var ok bool
	var err error
	if resp, ok = getCache(r); !ok {
		resp, err = dns.Exchange(r, s)
		if err != nil {
			return err
		}
		setCache(r.Question, resp)
	}

	return w.WriteMsg(resp)
}

func processProxy(w dns.ResponseWriter, r *dns.Msg, p, s string) error {
	var resp *dns.Msg
	var ok bool
	if resp, ok = getCache(r); !ok {
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

func local(w dns.ResponseWriter, r *dns.Msg) (err error) {
	_, err = executor.ExecuteConcurrentArg(
		strings.Split(*localDNS, ","),
		func(i interface{}) (interface{}, error) {
			err := process(w, r, i.(string))
			return nil, err
		},
	)
	return
}

func remote(w dns.ResponseWriter, r *dns.Msg) (err error) {
	if proxy := *socks5Proxy; proxy != "" {
		_, err = executor.ExecuteConcurrentArg(
			strings.Split(*remoteDNS, ","),
			func(i interface{}) (interface{}, error) {
				err := processProxy(w, r, proxy, i.(string))
				return nil, err
			},
		)
	} else {
		_, err = executor.ExecuteConcurrentArg(
			strings.Split(*remoteDNS, ","),
			func(i interface{}) (interface{}, error) {
				err := process(w, r, i.(string))
				return nil, err
			},
		)
	}

	return
}

var localHandler dns.HandlerFunc = func(w dns.ResponseWriter, r *dns.Msg) { local(w, r) }
var remoteHandler dns.HandlerFunc = func(w dns.ResponseWriter, r *dns.Msg) { remote(w, r) }

func main() {
	flag.Parse()

	remoteList, err := txt.ReadFile(*list)
	if err != nil {
		log.Print(err)
	}

	if *fallback {
		dns.DefaultServeMux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
			if _, err := executor.ExecuteSerial(
				nil,
				func(_ interface{}) (interface{}, error) {
					err := local(w, r)
					return nil, err
				},
				func(_ interface{}) (interface{}, error) {
					err := remote(w, r)
					return nil, err
				},
			); err != nil {
				log.Print(err)
			}
		})
		for _, i := range remoteList {
			dns.DefaultServeMux.HandleFunc(i, func(w dns.ResponseWriter, r *dns.Msg) {
				if _, err := executor.ExecuteSerial(
					nil,
					func(_ interface{}) (interface{}, error) {
						err := remote(w, r)
						return nil, err
					},
					func(_ interface{}) (interface{}, error) {
						err := local(w, r)
						return nil, err
					},
				); err != nil {
					log.Print(err)
				}
			})
		}
	} else {
		dns.DefaultServeMux.Handle(".", localHandler)
		for _, i := range remoteList {
			dns.DefaultServeMux.Handle(i, remoteHandler)
		}
	}

	log.Fatal(dns.ListenAndServe(":53", "udp", dns.DefaultServeMux))
}
