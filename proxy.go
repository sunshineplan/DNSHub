package main

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/sunshineplan/httpproxy"
	"golang.org/x/net/proxy"
)

var proxyList []*url.URL

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
			svc.Error("failed to parse url", "error", err)
			continue
		}
		if _, err := proxy.FromURL(u, nil); err != nil {
			svc.Error("failed to parse proxy", "error", err)
			continue
		}
		svc.Debug("proxy list", "index", len(proxyList)+1, "proxy", u)
		proxyList = append(proxyList, u)
	}
}

func parseProxy(s string) (string, *url.URL) {
	var n int
	for _, i := range s {
		if i == '*' {
			n++
		} else {
			break
		}
	}
	var u *url.URL
	if n > 0 && n <= len(proxyList) {
		svc.Debug("use proxy", "proxy", n)
		u = proxyList[n-1]
	}
	return strings.TrimLeft(s, "*"), u
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
