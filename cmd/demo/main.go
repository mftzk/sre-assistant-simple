// Command demo is a fake HTTP service used as a target in the sharing session.
// One binary, three behaviours chosen by MODE:
//
//	healthy  always 200
//	degrade  200 for DEGRADE_AFTER seconds, then 503 "db pool exhausted" until restarted
//	flaky    every 4th request hits a retry warning but still succeeds (restart is not the fix)
package main

import (
	"encoding/json"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	name := envOr("SERVICE_NAME", "demo")
	mode := envOr("MODE", "healthy")
	degradeAfter := 60 * time.Second
	if value, err := strconv.Atoi(os.Getenv("DEGRADE_AFTER")); err == nil && value > 0 {
		degradeAfter = time.Duration(value) * time.Second
	}

	start := time.Now()
	degraded := func() bool { return mode == "degrade" && time.Since(start) >= degradeAfter }
	var requests atomic.Int64

	http.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if degraded() {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	http.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		n := requests.Add(1)
		switch {
		case degraded():
			time.Sleep(2 * time.Second) // waiting for a connection that never frees up
			log.Printf("ERROR GET %s 503 %s db connection pool exhausted (50/50 in use, 0 idle)", r.URL.Path, since(begin))
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
			return
		case mode == "flaky" && n%4 == 0:
			log.Printf("WARN GET %s upstream smtp timeout, retry 1/3 in 500ms", r.URL.Path)
			time.Sleep(500 * time.Millisecond)
			log.Printf("INFO GET %s 200 %s sent after retry", r.URL.Path, since(begin))
		default:
			time.Sleep(time.Duration(10+rand.IntN(40)) * time.Millisecond)
			log.Printf("INFO GET %s 200 %s", r.URL.Path, since(begin))
		}
		writeJSON(w, http.StatusOK, map[string]any{"service": name, "request": n})
	})

	log.Printf("INFO %s listening on :8080 mode=%s", name, mode)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func since(t time.Time) time.Duration { return time.Since(t).Round(time.Millisecond) }

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
