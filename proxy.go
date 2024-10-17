package main

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/sunshineplan/httpproxy"
	"golang.org/x/net/proxy"
)

var proxyList []proxy.Dialer

func init() {
	proxy.RegisterDialerType("http", httpproxy.FromURL)
	proxy.RegisterDialerType("https", httpproxy.FromURL)
}

func initProxy() {
	for _, i := range strings.Split(*dnsProxy, ",") {
		if i = strings.TrimSpace(i); i == "" {
			continue
		}
		u, err := url.Parse(i)
		if err != nil {
			svc.Print(err)
			continue
		}
		p, err := proxy.FromURL(u, nil)
		if err != nil {
			svc.Print(err)
			continue
		}
		proxyList = append(proxyList, p)
	}
}

func parseProxy(s string) (string, proxy.Dialer) {
	var n int
	for _, i := range s {
		if i == '*' {
			n++
		} else {
			break
		}
	}
	var p proxy.Dialer
	if n > 0 && n <= len(proxyList) {
		p = proxyList[n-1]
	}
	return strings.TrimLeft(s, "*"), p
}

func dialContext(ctx context.Context, d proxy.Dialer, network, address string) (net.Conn, error) {
	var (
		conn net.Conn
		done = make(chan struct{}, 1)
		err  error
	)
	go func() {
		conn, err = d.Dial(network, address)
		close(done)
		if conn != nil && ctx.Err() != nil {
			conn.Close()
		}
	}()
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-done:
	}
	return conn, err
}
