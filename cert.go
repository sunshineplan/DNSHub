package main

import (
	"crypto/tls"
	"time"

	"github.com/sunshineplan/utils/cache"
)

var certCache = cache.NewWithRenew[string, *tls.Certificate](false)

func loadCertificate() (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(*cert, *privkey)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

func getCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	v, ok := certCache.Get("cert")
	if ok {
		return v, nil
	}
	cert, err := loadCertificate()
	if err != nil {
		return nil, err
	}
	certCache.Set("cert", cert, 24*time.Hour, loadCertificate)
	return cert, nil
}
