package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/miekg/dns"
	"golang.org/x/net/proxy"
)

type Client interface {
	ExchangeContext(context.Context, *dns.Msg) (*dns.Msg, error)
	Name() string
}

func parseClients(s string) (clients []Client) {
	for _, i := range strings.Split(s, ",") {
		if i = strings.TrimSpace(i); i == "" {
			continue
		}
		addr, proxyURL := parseProxy(i)
		addr = strings.ToLower(addr)
		if addr, ok := strings.CutSuffix(addr, "@doh"); ok {
			svc.Debug("found DNS over HTTPS", "address", addr)
			t := http.DefaultTransport.(*http.Transport).Clone()
			t.Proxy = http.ProxyURL(proxyURL)
			clients = append(clients, &doh{addr, &http.Client{Transport: t}})
			continue
		}
		c := &client{addr, new(dns.Client), nil}
		if proxyURL != nil {
			c.proxy, _ = proxy.FromURL(proxyURL, nil)
		}
		var ok bool
		if c.addr, ok = strings.CutSuffix(c.addr, "@tcp-tls"); ok {
			c.Net = "tcp-tls"
			servername, _, _ := net.SplitHostPort(c.addr)
			c.TLSConfig = &tls.Config{ServerName: servername}
		} else if c.addr, ok = strings.CutSuffix(c.addr, "@dot"); ok {
			c.Net = "tcp-tls"
			servername, _, _ := net.SplitHostPort(c.addr)
			c.TLSConfig = &tls.Config{ServerName: servername}
		} else if c.addr, ok = strings.CutSuffix(c.addr, "@tcp"); ok {
			c.Net = "tcp"
		}
		c.addr, _, _ = strings.Cut(c.addr, "@")
		if _, _, err := net.SplitHostPort(c.addr); err != nil {
			if c.Net == "tcp-tls" {
				c.addr += ":853"
			} else {
				c.addr += ":53"
			}
		}
		svc.Debug("found DNS", "network", c.Net, "address", c.addr)
		clients = append(clients, c)
	}
	return
}

type client struct {
	addr string
	*dns.Client
	proxy proxy.Dialer
}

func (c *client) ExchangeContext(ctx context.Context, m *dns.Msg) (r *dns.Msg, err error) {
	if c.proxy == nil {
		svc.Debug("direct", "DNS", c.addr, "request", m.Question)
		r, _, err = c.Client.ExchangeContext(ctx, m, c.addr)
	} else {
		network := c.Net
		if network == "" {
			network = "udp"
		}
		network = strings.TrimSuffix(network, "-tls")
		var conn net.Conn
		if d, ok := c.proxy.(proxy.ContextDialer); ok {
			conn, err = d.DialContext(ctx, network, c.addr)
		} else {
			conn, err = dialContext(ctx, c.proxy, network, c.addr)
		}
		if err != nil {
			return
		}
		if c.TLSConfig != nil {
			conn = tls.Client(conn, c.TLSConfig)
		}
		svc.Debug("proxy", "DNS", c.addr, "request", m.Question)
		r, _, err = c.ExchangeWithConnContext(ctx, r, &dns.Conn{Conn: conn})
	}
	return
}

func (c *client) Name() string {
	return c.addr
}

type doh struct {
	server string
	client *http.Client
}

func (c *doh) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	b, err := m.Pack()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://%s/dns-query", c.server), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	resp, err := c.client.Do(req)
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://%s/dns-query", c.server), bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/dns-message")
		var e error
		if resp, e = http.DefaultClient.Do(req); e != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	r := new(dns.Msg)
	if err := r.Unpack(b); err != nil {
		return nil, err
	}
	return r, nil
}

func (c *doh) Name() string {
	return c.server + "[DoH]"
}

var defaultResolver = &resolver{net.DefaultResolver}

type resolver struct {
	*net.Resolver
}

func (r resolver) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	svc.Debug("system DNS", "request", m.Question)
	m = m.Copy()
	q := m.Question[0].Name
	qType := m.Question[0].Qtype
	switch qType {
	case dns.TypeA, dns.TypeAAAA:
		ips, err := r.LookupIP(ctx, "ip", q)
		if err != nil {
			return nil, err
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
				return nil, err
			}
			if rr != nil {
				m.Answer = append(m.Answer, rr)
			}
		}
	case dns.TypeCNAME:
		cname, err := r.LookupCNAME(ctx, q)
		if err != nil {
			return nil, err
		}
		s := fmt.Sprintf("%s CNAME %s", q, cname)
		rr, err := dns.NewRR(s)
		if err != nil {
			return nil, err
		}
		m.Answer = append(m.Answer, rr)
	case dns.TypeTXT:
		txt, err := r.LookupTXT(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, i := range txt {
			s := fmt.Sprintf("%s TXT %q", q, i)
			rr, err := dns.NewRR(s)
			if err != nil {
				return nil, err
			}
			m.Answer = append(m.Answer, rr)
		}
	case dns.TypePTR:
		addr, err := r.LookupAddr(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, i := range addr {
			reverse, _ := dns.ReverseAddr(q)
			s := fmt.Sprintf("%s PTR %s", reverse, i)
			rr, err := dns.NewRR(s)
			if err != nil {
				return nil, err
			}
			m.Answer = append(m.Answer, rr)
		}
	case dns.TypeMX:
		mx, err := r.LookupMX(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, i := range mx {
			s := fmt.Sprintf("%s MX %d %s", q, i.Pref, i.Host)
			rr, err := dns.NewRR(s)
			if err != nil {
				return nil, err
			}
			m.Answer = append(m.Answer, rr)
		}
	case dns.TypeNS:
		ns, err := r.LookupNS(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, i := range ns {
			s := fmt.Sprintf("%s NS %s", q, i.Host)
			rr, err := dns.NewRR(s)
			if err != nil {
				return nil, err
			}
			m.Answer = append(m.Answer, rr)
		}
	case dns.TypeSRV:
		_, srv, err := r.LookupSRV(ctx, "", "", q)
		if err != nil {
			return nil, err
		}
		for _, i := range srv {
			s := fmt.Sprintf("%s SRV %d %d %d %s", q, i.Priority, i.Weight, i.Port, i.Target)
			rr, err := dns.NewRR(s)
			if err != nil {
				return nil, err
			}
			m.Answer = append(m.Answer, rr)
		}
	default:
		return nil, fmt.Errorf("not supported query type for local lookup: %d", qType)
	}
	return m, nil
}

func (resolver) Name() string {
	return "system"
}
