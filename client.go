package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	"github.com/miekg/dns"
	"golang.org/x/net/proxy"
)

type Client interface {
	ExchangeContext(context.Context, *dns.Msg) (*dns.Msg, error)
	Name() string
}

type client struct {
	addr string
	*dns.Client
	proxy proxy.Dialer
}

func parseClients(s string) (clients []Client) {
	for _, i := range strings.Split(s, ",") {
		if i = strings.TrimSpace(i); i == "" {
			continue
		}
		addr, dialer := parseProxy(i)
		c := &client{addr, new(dns.Client), dialer}
		var ok bool
		if c.addr, ok = strings.CutSuffix(c.addr, "@tcp-tls"); ok {
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

func (c *client) ExchangeContext(ctx context.Context, m *dns.Msg) (r *dns.Msg, err error) {
	if c.proxy == nil {
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
		r, _, err = c.ExchangeWithConnContext(ctx, r, &dns.Conn{Conn: conn})
	}
	return
}

func (c *client) Name() string {
	return c.addr
}

var defaultResolver = &resolver{net.DefaultResolver}

type resolver struct {
	*net.Resolver
}

func (r resolver) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
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
