package main

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/miekg/dns"
	"github.com/sunshineplan/utils/txt"
)

var mu sync.Mutex
var remoteList []string

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
			log.Println("failed to load remote list file:", err)
		} else {
			registerHandler()
		}

		w, err := fsnotify.NewWatcher()
		if err != nil {
			log.Print(err)
			return
		}
		if err = w.Add(*list); err != nil {
			log.Print(err)
			return
		}

		go func() {
			for {
				event, ok := <-w.Events
				if !ok {
					return
				}

				switch event.Op.String() {
				case "WRITE", "CREATE":
					mu.Lock()
					reRegisterHandler()
					mu.Unlock()
				case "REMOVE", "RENAME":
					mu.Lock()
					for _, i := range remoteList {
						dns.DefaultServeMux.HandleRemove(dns.Fqdn(i))
					}
					mu.Unlock()
				}
			}
		}()
	} else {
		registerHandler()
	}
}
