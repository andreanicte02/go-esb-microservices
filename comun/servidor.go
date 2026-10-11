package comun

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

// Servidor runs an HTTP server that shuts down gracefully.
//
// When Kubernetes replaces a pod it sends SIGTERM and, at the same time, starts
// removing the pod from its Service. That removal takes a moment to reach every
// node, so for a few seconds traffic still arrives. Exiting on SIGTERM (Go's
// default) turns those requests into "connection refused". Instead the server
// reports not-ready, keeps serving for DRAIN_SECONDS, then closes cleanly.
type Servidor struct {
	srv      *http.Server
	drenando atomic.Bool
	drenaje  time.Duration
}

func NuevoServidor(addr string, h http.Handler) *Servidor {
	drenaje := 5 * time.Second
	if s, err := strconv.Atoi(os.Getenv("DRAIN_SECONDS")); err == nil && s >= 0 {
		drenaje = time.Duration(s) * time.Second
	}
	return &Servidor{
		srv:     &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second},
		drenaje: drenaje,
	}
}

// Drenando reports whether the server received SIGTERM and is on its way out.
func (s *Servidor) Drenando() bool { return s.drenando.Load() }

// Correr serves until SIGTERM/SIGINT, then drains and shuts down.
func (s *Servidor) Correr() {
	errCh := make(chan error, 1)
	go func() { errCh <- s.srv.ListenAndServe() }()

	señal := make(chan os.Signal, 1)
	signal.Notify(señal, syscall.SIGTERM, syscall.SIGINT)
	select {
	case err := <-errCh:
		log.Fatal(err)
	case sig := <-señal:
		log.Printf("%v: dejando de recibir tráfico, apagado en %v", sig, s.drenaje)
	}
	s.drenando.Store(true)
	time.Sleep(s.drenaje)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("apagado: %v", err)
	}
}
