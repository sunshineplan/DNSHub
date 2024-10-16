package main

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/miekg/dns"
)

type Client interface {
	ExchangeContext(context.Context, *dns.Msg) (*dns.Msg, error)
}

type client struct {
	addr string
	*dns.Client
}

func parseClients(s string) (clients []Client) {
	for _, i := range strings.Split(s, ",") {
		if i = strings.TrimSpace(i); i == "" {
			continue
		}
		c := &client{i, new(dns.Client)}
		var ok bool
		if c.addr, ok = strings.CutSuffix(c.addr, "@tcp-tls"); ok {
			c.Net = "tcp-tls"
		} else if c.addr, ok = strings.CutSuffix(c.addr, "@tcp"); ok {
			c.Net = "tcp"
		}
		c.addr, _, _ = strings.Cut(c.addr, "@")
		if _, _, err := net.SplitHostPort(c.addr); err != nil {
			if c.Net == "tcp-tls" {
				c.addr += "853"
			} else {
				c.addr += ":domain"
			}
		}
		clients = append(clients, c)
	}
	return
}

func (c *client) ExchangeContext(ctx context.Context, m *dns.Msg) (r *dns.Msg, err error) {
	r, _, err = c.Client.ExchangeContext(ctx, m, c.addr)
	return
}

var defaultClient = localResolver{net.DefaultResolver}

type localResolver struct {
	*net.Resolver
}

func (r localResolver) ExchangeContext(ctx context.Context, m *dns.Msg) (*dns.Msg, error) {
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
