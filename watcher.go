package main

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/miekg/dns"
	"github.com/sunshineplan/utils/txt"
)

var (
	mu         sync.Mutex
	remoteList []string
)

func initRemoteList() {
	if *list == "" {
		if info, err := os.Stat(filepath.Join(filepath.Dir(self), "remote.list")); err == nil && !info.IsDir() {
			*list = filepath.Join(filepath.Dir(self), "remote.list")
		}
	}

	var err error
	if *list != "" {
		remoteList, err = txt.ReadFile(*list)
		if err != nil {
			svc.Println("failed to load remote list file:", err)
		} else {
			registerHandler()
		}

		w, err := fsnotify.NewWatcher()
		if err != nil {
			svc.Print(err)
			return
		}
		if err = w.Add(filepath.Dir(*list)); err != nil {
			svc.Print(err)
			return
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
					if event.Name == *list {
						switch {
						case event.Has(fsnotify.Create), event.Has(fsnotify.Write):
							mu.Lock()
							reRegisterHandler()
							mu.Unlock()
						case event.Has(fsnotify.Remove), event.Has(fsnotify.Rename):
							mu.Lock()
							for _, i := range remoteList {
								dns.DefaultServeMux.HandleRemove(dns.Fqdn(i))
							}
							remoteList = nil
							mu.Unlock()
						}
					}
				}
			}
		}()
	} else {
		registerHandler()
	}
}
