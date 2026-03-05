package main

import (
	"fmt"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/sunshineplan/utils/cache"
)

var dnsCache = cache.NewWithRenew[string, *dns.Msg](true)

func getCache(key []dns.RR) (*dns.Msg, bool) {
	if m, ok := dnsCache.Get(fmt.Sprint(key)); ok {
		return m.Copy(), true
	}
	return nil, false
}

func setCache(key []dns.RR, r *dns.Msg) {
	m := r.Copy()
	m.ID = 0
	m.Data = nil
	lifecycle := 300 * time.Second
	if len(r.Answer) > 0 {
		lifecycle = time.Duration(max(r.Answer[0].Header().TTL, 300)) * time.Second
	}
	dnsCache.Set(fmt.Sprint(key), m, lifecycle, nil)
}
