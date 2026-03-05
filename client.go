package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnshttp"
	"codeberg.org/miekg/dns/dnsutil"
	"golang.org/x/net/proxy"
)

type Client interface {
	ExchangeContext(context.Context, *dns.Msg) (*dns.Msg, error)
	Name() string
}

func parseClients(s string) (clients []Client) {
	for i := range strings.SplitSeq(s, ",") {
		if i = strings.TrimSpace(i); i == "" {
			continue
		}
		addr, proxyURL := parseProxy(i)
		addr = strings.ToLower(addr)
		if addr, ok := strings.CutSuffix(addr, "@doh"); ok {
			svc.Debug("found DNS over HTTPS", "address", addr)
			t := http.DefaultTransport.(*http.Transport).Clone()
			if proxyURL != nil {
				t.Proxy = http.ProxyURL(proxyURL)
			}
			clients = append(clients, &doh{addr, &http.Client{Transport: t}})
			continue
		}
		c := &client{"udp", addr, dns.NewClient(), nil}
		if proxyURL != nil {
			c.proxy, _ = proxy.FromURL(proxyURL, nil)
		}
		var ok bool
		if c.address, ok = strings.CutSuffix(c.address, "@dot"); ok {
			c.network = "tcp"
			servername, _, _ := net.SplitHostPort(c.address)
			c.TLSConfig = &tls.Config{ServerName: servername}
		} else if c.address, ok = strings.CutSuffix(c.address, "@tcp"); ok {
			c.network = "tcp"
		}
		if _, _, err := net.SplitHostPort(c.address); err != nil {
			if c.TLSConfig != nil {
				c.address += ":853"
			} else {
				c.address += ":53"
			}
		}
		svc.Debug("found DNS", "network", c.network, "address", c.address)
		clients = append(clients, c)
	}
	return
}

type client struct {
	network string
	address string
	*dns.Client
	proxy proxy.Dialer
}

func (c *client) ExchangeContext(ctx context.Context, m *dns.Msg) (r *dns.Msg, err error) {
	if c.proxy == nil {
		svc.Debug("direct", "DNS", c.address, "request", m.Question)
		r, _, err = c.Client.Exchange(ctx, m, c.network, c.address)
	} else {
		var conn net.Conn
		if d, ok := c.proxy.(proxy.ContextDialer); ok {
			conn, err = d.DialContext(ctx, c.network, c.address)
		} else {
			conn, err = dialContext(ctx, c.proxy, c.network, c.address)
		}
		if err != nil {
			return
		}
		if c.TLSConfig != nil {
			conn = tls.Client(conn, c.TLSConfig)
		}
		svc.Debug("proxy", "DNS", c.address, "request", m.Question)
		r, _, err = c.ExchangeWithConn(ctx, r, conn)
	}
	return
}

func (c *client) Name() string {
	return c.address
}

type doh struct {
	server string
	client *http.Client
}

func (c *doh) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
	req, err := dnshttp.NewRequest(http.MethodPost, "https://"+c.server, m.Copy())
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req.WithContext(ctx))
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		req, _ := dnshttp.NewRequest(http.MethodPost, "https://"+c.server, m.Copy())
		var e error
		if resp, e = http.DefaultClient.Do(req.WithContext(ctx)); e != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	r, err := dnshttp.Response(resp)
	if err != nil {
		return nil, err
	}
	r.Data = nil
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
	q := m.Question[0].Header().Name
	qType := dns.RRToType(m.Question[0])
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
			rr, err := dns.New(s)
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
		rr, err := dns.New(s)
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
			rr, err := dns.New(s)
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
		reverse := dnsutil.ReverseAddr(netip.MustParseAddr(q))
		for _, i := range addr {
			s := fmt.Sprintf("%s PTR %s", reverse, i)
			rr, err := dns.New(s)
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
			rr, err := dns.New(s)
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
			rr, err := dns.New(s)
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
			rr, err := dns.New(s)
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
