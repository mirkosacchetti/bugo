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
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	ch := make(chan string, 8)
	sseClientsMu.Lock()
	sseClients[ch] = struct{}{}
	sseClientsMu.Unlock()

	defer func() {
		sseClientsMu.Lock()
		delete(sseClients, ch)
		sseClientsMu.Unlock()
	}()

	// Each write is bounded by a deadline: if a client stops reading (e.g. a
	// reloaded page whose connection the browser keeps alive for reuse), the
	// write fails instead of pinning this goroutine and its connection slot
	// forever, and the handler returns to free it.
	write := func(s string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := fmt.Fprint(w, s); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !write(": connected\n\n") {
		return
	}

	// Heartbeat: keep the connection alive while idle and surface a dead client
	// promptly via the bounded write above.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !write(": ping\n\n") {
				return
			}
		case msg := <-ch:
			if !write("data: " + msg + "\n\n") {
				return
			}
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
		// Trailing-edge debounce: a single save fires a burst of events
		// (truncate, write, chmod, rename). Coalesce them and broadcast once,
		// after ~150ms of quiet, so the reload sees the finished file.
		const debounce = 150 * time.Millisecond
		timer := time.NewTimer(0)
		<-timer.C // drain the immediate fire; timer is now idle until Reset
		pending := false
		cssOnly := true
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
				isCSS := strings.EqualFold(filepath.Ext(ev.Name), ".css")
				if pending {
					cssOnly = cssOnly && isCSS
				} else {
					cssOnly = isCSS
					pending = true
				}
				log.Printf("changed: %s", ev.Name)
				timer.Reset(debounce)
			case <-timer.C:
				if !pending {
					continue
				}
				pending = false
				msg := "reload"
				if cssOnly {
					msg = "css"
				}
				log.Printf("reload -> %s", msg)
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
