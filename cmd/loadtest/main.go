// Command loadtest spins up N synthetic clients that each open the SSE stream
// and POST randomized drag input, so the server can be stressed without
// dozens of real browsers.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	target := flag.String("url", "http://localhost:8080", "server base URL")
	clients := flag.Int("clients", 50, "number of synthetic clients")
	rate := flag.Int("rate", 40, "input messages per second per client")
	dur := flag.Duration("duration", 30*time.Second, "test duration")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *dur)
	defer cancel()

	var bytesRead int64
	var wg sync.WaitGroup
	for i := 0; i < *clients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runClient(ctx, *target, *rate, id, &bytesRead)
		}(i)
	}

	// Periodic throughput report.
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		var prev int64
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				now := atomic.LoadInt64(&bytesRead)
				fmt.Printf("clients=%d  stream=%.2f MB/s\n", *clients, float64(now-prev)/2/1e6)
				prev = now
			}
		}
	}()

	wg.Wait()
	fmt.Printf("done. total stream bytes received: %.2f MB\n", float64(atomic.LoadInt64(&bytesRead))/1e6)
}

func runClient(ctx context.Context, base string, rate, id int, bytesRead *int64) {
	rng := rand.New(rand.NewSource(int64(id) + time.Now().UnixNano()))
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar}

	// Prime the session cookie via the index page.
	if req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/", nil); err == nil {
		if resp, err := hc.Do(req); err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}

	// Open the SSE stream and count bytes.
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/stream", nil)
		if err != nil {
			return
		}
		resp, err := hc.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		buf := make([]byte, 32*1024)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				atomic.AddInt64(bytesRead, int64(n))
			}
			if err != nil {
				return
			}
		}
	}()

	// Drive randomized drag input.
	interval := time.Second / time.Duration(rate)
	t := time.NewTicker(interval)
	defer t.Stop()
	x, y := rng.Float64()*1920, rng.Float64()*1080
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			nx := x + (rng.Float64()*2-1)*60
			ny := y + (rng.Float64()*2-1)*60
			body := fmt.Sprintf(`{"x":%.1f,"y":%.1f,"dvx":%.1f,"dvy":%.1f,"active":true}`,
				nx, ny, (nx-x)*rate2(rate), (ny-y)*rate2(rate))
			x, y = nx, ny
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/input", bytes.NewBufferString(body))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			if resp, err := hc.Do(req); err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
	}
}

func rate2(rate int) float64 { return float64(rate) }
