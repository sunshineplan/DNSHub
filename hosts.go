package main

import (
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/miekg/dns"
	"github.com/sunshineplan/utils/txt"
)

func parseHosts(file string) {
	file = trim(file)
	if file == "" {
		return
	}

	rows, err := txt.ReadFile(file)
	if err != nil {
		log.Print(err)
		return
	}

	ipv4 := make(map[string][]net.IP)
	ipv6 := make(map[string][]net.IP)
	for line, i := range rows {
		elem := fmtHostsRow(i)
		if l := len(elem); l == 0 {
			continue
		} else if l < 2 {
			log.Printf("illegal hosts row: line %d: %s", line, i)
			continue
		}
		ip := net.ParseIP(elem[0])
		if ip == nil {
			log.Printf("illegal hosts row: line %d: %s", line, i)
			continue
		}
		ipMap := ipv4
		if ip.DefaultMask() == nil {
			ipMap = ipv6
		}
		for index, i := range elem {
			if index == 0 {
				continue
			}
			ipMap[i] = append(ipMap[i], ip)
		}
	}

	importHosts(ipv4, dns.TypeA)
	importHosts(ipv6, dns.TypeAAAA)
}

func importHosts(s map[string][]net.IP, t uint16) {
	qType := "A"
	if t == dns.TypeAAAA {
		qType = "AAAA"
	}

	for k, v := range s {
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(k), t)
		for _, ip := range v {
			s := fmt.Sprintf("%s %s %s", dns.Fqdn(k), qType, ip)
			rr, err := dns.NewRR(s)
			if err != nil {
				log.Println("failed to create record:", s)
				continue
			}
			m.Answer = append(m.Answer, rr)
		}
		dnsCache.Set(fmt.Sprint(m.Question), m, 0, nil)
	}
}

func fmtHostsRow(row string) []string {
	if i := strings.IndexRune(row, '#'); i != -1 {
		row = row[:i]
	}
	return strings.Fields(row)
}
