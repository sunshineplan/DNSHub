package main

import (
	"context"

	"github.com/miekg/dns"
	"github.com/sunshineplan/workers/executor"
)

var dnsExecutor = executor.New[Client, *dns.Msg](0)

func ExchangeContext(ctx context.Context, r *dns.Msg, clients []Client) (*dns.Msg, error) {
	return dnsExecutor.ExecuteConcurrentArg(
		clients,
		func(c Client) (*dns.Msg, error) { return c.ExchangeContext(ctx, r) },
	)
}

func initHandler(clients, backup []Client) {
	dns.DefaultServeMux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		if m, ok := getCache(r); ok {
			w.WriteMsg(m)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		m, err := ExchangeContext(ctx, r, clients)
		if err != nil {
			svc.Print(err)
			if *fallback {
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				defer cancel()
				if m, err = ExchangeContext(ctx, r, backup); err != nil {
					svc.Print(err)
				}
			}
		}
		if err != nil {
			return
		}
		setCache(r.Question, m)
		w.WriteMsg(m)
	})
}

func registerExclude(old, new []string, clients []Client) {
	for _, i := range old {
		dns.DefaultServeMux.HandleRemove(dns.Fqdn(i))
	}
	for _, i := range new {
		dns.DefaultServeMux.HandleFunc(dns.Fqdn(i), func(w dns.ResponseWriter, r *dns.Msg) {
			if m, ok := getCache(r); ok {
				w.WriteMsg(m)
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), *timeout)
			defer cancel()
			m, err := ExchangeContext(ctx, r, clients)
			if err != nil {
				svc.Print(err)
				return
			}
			setCache(r.Question, m)
			w.WriteMsg(m)
		})
	}
}
