package main

import (
	"context"
	"errors"

	"github.com/miekg/dns"
	"github.com/sunshineplan/workers/executor"
)

type Result struct {
	msg  *dns.Msg
	name string
}

func ExchangeContext(ctx context.Context, r *dns.Msg, clients ...Client) (*Result, error) {
	n := len(clients)
	if n == 0 {
		return nil, errors.New("no DNS clients")
	}
	return executor.Executor[Client, *Result](n).ExecuteConcurrentArg(
		clients,
		func(c Client) (*Result, error) {
			m, err := c.ExchangeContext(ctx, r)
			if err != nil {
				return nil, err
			}
			return &Result{m, c.Name()}, nil
		},
	)
}

func initHandle(primary, backup []Client) {
	dns.DefaultServeMux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		svc.Debug("request", "local", w.LocalAddr(), "remote", w.RemoteAddr(), "question", r.Question)
		if m, ok := getCache(r); ok {
			svc.Debug("cached", "question", r.Question, "result", m)
			w.WriteMsg(m)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		m, err := ExchangeContext(ctx, r, primary...)
		if err != nil {
			svc.Error("request failed", "error", err)
			if *fallback {
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				defer cancel()
				if m, err = ExchangeContext(ctx, r, backup...); err != nil {
					svc.Error("fallback backup request failed", "error", err)
					ctx, cancel := context.WithTimeout(context.Background(), *timeout)
					defer cancel()
					if m, err = ExchangeContext(ctx, r, defaultResolver); err != nil {
						svc.Error("fallback system request failed", "error", err)
					}
				}
			}
		}
		if err != nil {
			return
		}
		svc.Debug("uncached", "DNS", m.name, "question", r.Question, "result", m.msg)
		setCache(r.Question, m.msg)
		w.WriteMsg(m.msg)
	})
}

func registerExclude(old, new []string, primary, backup []Client) {
	svc.Debug("register exclude handle")
	for _, i := range old {
		svc.Debug("remove", "pattern", i)
		dns.DefaultServeMux.HandleRemove(dns.Fqdn(i))
	}
	for _, i := range new {
		svc.Debug("add", "pattern", i)
		dns.DefaultServeMux.HandleFunc(dns.Fqdn(i), func(w dns.ResponseWriter, r *dns.Msg) {
			svc.Debug("request exclude", "local", w.LocalAddr(), "remote", w.RemoteAddr(), "question", r.Question)
			if m, ok := getCache(r); ok {
				svc.Debug("cached", "question", r.Question, "result", m)
				w.WriteMsg(m)
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), *timeout)
			defer cancel()
			m, err := ExchangeContext(ctx, r, backup...)
			if err != nil {
				svc.Error("request exclude failed", "error", err)
				if *fallback {
					ctx, cancel := context.WithTimeout(context.Background(), *timeout)
					defer cancel()
					if m, err = ExchangeContext(ctx, r, primary...); err != nil {
						svc.Error("fallback primary request exclude failed", "error", err)
						ctx, cancel := context.WithTimeout(context.Background(), *timeout)
						defer cancel()
						if m, err = ExchangeContext(ctx, r, defaultResolver); err != nil {
							svc.Error("fallback system request exclude failed", "error", err)
						}
					}
				}
			}
			if err != nil {
				return
			}
			svc.Debug("uncached", "DNS", m.name, "question", r.Question, "result", m.msg)
			setCache(r.Question, m.msg)
			w.WriteMsg(m.msg)
		})
	}
}
