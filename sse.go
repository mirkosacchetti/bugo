package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var (
	sseClients   = make(map[chan string]struct{})
	sseClientsMu sync.Mutex
)

func sseHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 8)
	sseClientsMu.Lock()
	sseClients[ch] = struct{}{}
	sseClientsMu.Unlock()

	defer func() {
		sseClientsMu.Lock()
		delete(sseClients, ch)
		sseClientsMu.Unlock()
	}()

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func broadcast(msg string) {
	sseClientsMu.Lock()
	defer sseClientsMu.Unlock()
	for ch := range sseClients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func startWatcher(dirs ...string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	for _, d := range dirs {
		err := filepath.WalkDir(d, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if addErr := watcher.Add(path); addErr != nil {
					log.Printf("watch %s: %v", path, addErr)
				}
			}
			return nil
		})
		if err != nil {
			log.Printf("walk %s: %v", d, err)
		}
	}

	go func() {
		var last time.Time
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
					continue
				}
				if info, statErr := os.Stat(ev.Name); statErr == nil && info.IsDir() && ev.Op&fsnotify.Create != 0 {
					_ = watcher.Add(ev.Name)
				}
				if time.Since(last) < 100*time.Millisecond {
					continue
				}
				last = time.Now()

				msg := "reload"
				if strings.EqualFold(filepath.Ext(ev.Name), ".css") {
					msg = "css"
				}
				log.Printf("changed: %s -> %s", ev.Name, msg)
				broadcast(msg)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("watcher error: %v", err)
			}
		}
	}()
	return nil
}
