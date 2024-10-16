package main

import (
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/miekg/dns"
	"github.com/sunshineplan/utils/txt"
)

func initExcludeList(file string, clients []Client) []string {
	exclude, err := txt.ReadFile(file)
	if err != nil {
		svc.Println("failed to load exclude list file:", err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		svc.Print(err)
		return nil
	}
	if err = w.Add(filepath.Dir(file)); err != nil {
		svc.Print(err)
		return nil
	}

	go func() {
		for {
			select {
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				svc.Print(err)
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if event.Name == file {
					switch {
					case event.Has(fsnotify.Create), event.Has(fsnotify.Write):
						s, err := txt.ReadFile(file)
						if err != nil {
							svc.Println("failed to load exclude list file:", err)
						} else {
							registerExclude(exclude, s, clients)
							exclude = s
						}
					case event.Has(fsnotify.Remove), event.Has(fsnotify.Rename):
						for _, i := range exclude {
							dns.DefaultServeMux.HandleRemove(dns.Fqdn(i))
						}
						exclude = nil
					}
				}
			}
		}
	}()

	return exclude
}
