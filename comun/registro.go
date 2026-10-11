// Package comun has what every microservice needs to live behind the ESB in
// Kubernetes: registering with the bus (and staying registered), and health checks.
package comun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

// Operacion is one route a service exposes through the ESB.
type Operacion struct {
	Nombre string
	Ruta   string
	Method string
}

// descriptor is the body of POST /registrar_microservicio. Host, Nombre, Ruta,
// Padre and Method are the 2021 format; Url is how the ESB reaches this service.
type descriptor struct {
	Host   string
	Nombre string
	Ruta   string
	Padre  string
	Method string
	Url    string `json:",omitempty"`
}

// Registro keeps a service registered with the ESB.
//
// In 2021 each service registered once at startup. In Kubernetes that breaks in
// two ways: the ESB may not be up yet (the first request fails), and when the
// ESB pod restarts it forgets everyone, because its registry lives in memory.
// So registration is retried with backoff and then repeated on every heartbeat;
// the ESB treats repeats as no-ops.
type Registro struct {
	ESB         string // base URL of the bus, e.g. http://esb:8085
	Padre       string // this service's name, e.g. "cliente"
	Puerto      string // port it listens on, e.g. "8080"
	Url         string // how the ESB reaches it, e.g. http://cliente:8080
	Operaciones []Operacion
	Intervalo   time.Duration // time between heartbeats

	listo atomic.Bool
	http  http.Client

	// Drenando, when set, makes the readiness probe fail during graceful shutdown.
	Drenando func() bool
}

// NuevoRegistro builds a Registro from the environment:
// ESB_URL, SERVICE_URL and HEARTBEAT_SECONDS, with defaults that match docker-compose.
func NuevoRegistro(padre, puerto string, ops ...Operacion) *Registro {
	intervalo := 5 * time.Second
	if s := os.Getenv("HEARTBEAT_SECONDS"); s != "" {
		if d, err := time.ParseDuration(s + "s"); err == nil && d > 0 {
			intervalo = d
		}
	}
	return &Registro{
		ESB:         Env("ESB_URL", "http://esb:8085"),
		Padre:       padre,
		Puerto:      puerto,
		Url:         Env("SERVICE_URL", "http://"+padre+":"+puerto),
		Operaciones: ops,
		Intervalo:   intervalo,
		http:        http.Client{Timeout: 3 * time.Second},
	}
}

// Iniciar registers in the background until ctx is cancelled.
func (r *Registro) Iniciar(ctx context.Context) {
	go func() {
		espera := 500 * time.Millisecond
		for {
			if err := r.registrarTodo(ctx); err != nil {
				if r.listo.Swap(false) {
					log.Printf("registro: se perdió el ESB: %v", err)
				}
				espera = min(espera*2, 10*time.Second) // backoff while the ESB is down
			} else {
				if !r.listo.Swap(true) {
					log.Printf("registro: %d operaciones de %s registradas en %s", len(r.Operaciones), r.Padre, r.ESB)
				}
				espera = r.Intervalo
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(espera):
			}
		}
	}()
}

func (r *Registro) registrarTodo(ctx context.Context) error {
	for _, op := range r.Operaciones {
		body, _ := json.Marshal(descriptor{r.Puerto, op.Nombre, op.Ruta, r.Padre, op.Method, r.Url})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.ESB+"/registrar_microservicio", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := r.http.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("ESB respondió %s", resp.Status)
		}
	}
	return nil
}

// Listo reports whether the last registration attempt succeeded.
func (r *Registro) Listo() bool { return r.listo.Load() }

// Vivo is the liveness probe: the process is up and serving HTTP.
func Vivo(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("ok\n"))
}

// ListoHandler is the readiness probe: only route traffic here once the ESB knows about us.
func (r *Registro) ListoHandler(w http.ResponseWriter, _ *http.Request) {
	if r.Drenando != nil && r.Drenando() {
		http.Error(w, "apagándose\n", http.StatusServiceUnavailable)
		return
	}
	if !r.Listo() {
		http.Error(w, "aún no registrado en el ESB\n", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok\n"))
}

// Env returns the environment variable, or def when it is unset.
func Env(clave, def string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return def
}
