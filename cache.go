package main

import (
	"fmt"
	"time"

	"github.com/miekg/dns"
	"github.com/sunshineplan/utils/cache"
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

func setCache(key any, r *dns.Msg) {
	if len(r.Answer) == 0 {
		dnsCache.Set(fmt.Sprint(key), r, 300*time.Second, nil)
		return
	}

	ttl := r.Answer[0].Header().Ttl
	if ttl == 0 {
		ttl = 300
	}
	dnsCache.Set(fmt.Sprint(key), r, time.Duration(ttl)*time.Second, nil)
}
