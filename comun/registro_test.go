package comun

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeESB records registrations and can be switched off to simulate a restart.
type fakeESB struct {
	mu       sync.Mutex
	vistos   map[string]descriptor
	apagado  atomic.Bool
	llamadas atomic.Int32
}

func (f *fakeESB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.llamadas.Add(1)
	if f.apagado.Load() {
		http.Error(w, "down", http.StatusServiceUnavailable)
		return
	}
	var d descriptor
	json.NewDecoder(r.Body).Decode(&d)
	f.mu.Lock()
	f.vistos[d.Padre+"/"+d.Nombre] = d
	f.mu.Unlock()
}

func (f *fakeESB) olvidar() {
	f.mu.Lock()
	f.vistos = map[string]descriptor{}
	f.mu.Unlock()
}

func (f *fakeESB) cuantos() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.vistos)
}

func esperar(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timeout: " + msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRegistraYSobreviveReinicioDelESB(t *testing.T) {
	esb := &fakeESB{vistos: map[string]descriptor{}}
	esb.apagado.Store(true) // the ESB is not up yet when the service starts
	srv := httptest.NewServer(esb)
	defer srv.Close()

	r := &Registro{ESB: srv.URL, Padre: "cliente", Puerto: "8080", Url: "http://cliente:8080",
		Operaciones: []Operacion{{"solicitar_pedido", "/solicitar_pedido", "POST"}, {"get_estado", "/get_estado", "GET"}},
		Intervalo:   50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Iniciar(ctx)

	esperar(t, func() bool { return esb.llamadas.Load() > 0 }, "first attempt")
	if r.Listo() {
		t.Fatal("ready while the ESB is down")
	}

	esb.apagado.Store(false)
	esperar(t, func() bool { return r.Listo() && esb.cuantos() == 2 }, "registration after the ESB comes up")
	if d := esb.vistos["cliente/solicitar_pedido"]; d.Url != "http://cliente:8080" || d.Host != "8080" {
		t.Fatalf("unexpected descriptor: %+v", d)
	}

	esb.olvidar() // ESB pod restarted with an empty registry
	esperar(t, func() bool { return esb.cuantos() == 2 }, "re-registration on heartbeat")
}

func TestHandlersDeSalud(t *testing.T) {
	r := &Registro{}
	rec := httptest.NewRecorder()
	r.ListoHandler(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz before registering = %d, want 503", rec.Code)
	}
	r.listo.Store(true)
	rec = httptest.NewRecorder()
	r.ListoHandler(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz after registering = %d, want 200", rec.Code)
	}
	rec = httptest.NewRecorder()
	Vivo(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}
}
