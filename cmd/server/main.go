// Command server runs the physics sandbox: it starts the simulation hub and
// serves the web UI, SSE frame stream, and input endpoints.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/AndyNormann/physics-sandbox-go/assets"
	"github.com/AndyNormann/physics-sandbox-go/internal/hub"
	"github.com/AndyNormann/physics-sandbox-go/internal/web"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	particles := flag.Int("particles", 5000, "initial particle count")
	flag.Parse()

	h := hub.New(*particles)
	stop := make(chan struct{})
	go h.Run(stop)

	srv := web.New(h, assets.Static())
	httpSrv := &http.Server{Addr: *addr, Handler: srv.Routes()}

	go func() {
		log.Printf("physics sandbox listening on %s (%d particles)", *addr, *particles)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	close(stop)
	log.Println("shutting down")
}
