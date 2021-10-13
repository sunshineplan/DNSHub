package main

import (
	"log"
	"sync"
	"time"

	"github.com/sunshineplan/utils/txt"
	"github.com/sunshineplan/utils/watcher"
)

var mu sync.Mutex
var remoteList []string

func initRemoteList() {
	var err error
	if *list != "" {
		remoteList, err = txt.ReadFile(*list)
		if err != nil {
			log.Println("failed to load remote list file:", err)
			return
		}
		registerHandler()

		w := watcher.New(*list, time.Second)
		go func() {
			for {
				<-w.C

				mu.Lock()
				reRegisterHandler()
				mu.Unlock()
			}
		}()
	}
}
