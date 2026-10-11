package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func registrar(t *testing.T, h http.Handler, s ServicioData) {
	t.Helper()
	body, _ := json.Marshal(s)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/registrar_microservicio", strings.NewReader(string(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
}

func TestReRegistrarNoDuplica(t *testing.T) {
	reg := nuevoRegistro()
	h := router(reg, func() bool { return false }, 0)
	for i := 0; i < 3; i++ { // the same service heartbeating three times
		registrar(t, h, ServicioData{Host: "8082", Nombre: "marcar_pedido", Ruta: "/marcar_pedido", Padre: "repartidor", Method: "POST"})
	}
	if n := len(reg.lista()); n != 1 {
		t.Fatalf("registry has %d entries, want 1", n)
	}
}

func TestRegistroConcurrente(t *testing.T) {
	reg := nuevoRegistro()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); reg.guardar(ServicioData{Padre: "a", Nombre: "x"}) }()
		go func() { defer wg.Done(); reg.buscar("a", "x") }()
	}
	wg.Wait() // run with -race
}

func TestReenviaElCuerpoTalCual(t *testing.T) {
	var recibido string
	destinoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		recibido = string(b)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"message":"ok","id":7}`))
	}))
	defer destinoSrv.Close()

	h := router(nuevoRegistro(), func() bool { return false }, 0)
	registrar(t, h, ServicioData{Host: "8082", Nombre: "marcar_pedido", Ruta: "/marcar_pedido",
		Padre: "repartidor", Method: "POST", Url: destinoSrv.URL})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/repartidor_marcar_pedido", strings.NewReader(`{"idPedido":7}`)))
	if recibido != `{"idPedido":7}` {
		t.Fatalf("service received %q; fields must not be dropped", recibido)
	}
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"id":7`) {
		t.Fatalf("relayed %d %s", rec.Code, rec.Body)
	}
}

func TestErrores(t *testing.T) {
	h := router(nuevoRegistro(), func() bool { return false }, 0)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/cliente_solicitar_pedido", strings.NewReader(`{"id":1}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unregistered service: %d, want 503", rec.Code)
	}

	registrar(t, h, ServicioData{Host: "8080", Nombre: "solicitar_pedido", Ruta: "/x",
		Padre: "cliente", Method: "POST", Url: "http://127.0.0.1:1"})
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/cliente_solicitar_pedido", strings.NewReader(`{"id":1}`)))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("unreachable service: %d, want 502", rec.Code)
	}
}

func TestDestinoSinUrlUsaElMapeoDe2021(t *testing.T) {
	if got := destino(ServicioData{Host: "8081", Ruta: "/estado_pedido"}); got != "http://restaurante:8081/estado_pedido" {
		t.Fatalf("destino = %s", got)
	}
}

func TestReintentaSoloSiLaConexionFueRechazada(t *testing.T) {
	// Reserve a port with nothing listening, then start the service there after a moment.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	var llamadas atomic.Int32
	go func() {
		time.Sleep(250 * time.Millisecond)
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			llamadas.Add(1)
			b, _ := io.ReadAll(r.Body)
			w.Write(b)
		}))
	}()

	h := router(nuevoRegistro(), func() bool { return false }, 0)
	registrar(t, h, ServicioData{Host: "8081", Nombre: "recibir_pedido", Ruta: "/recibir_pedido",
		Padre: "restaurante", Method: "POST", Url: "http://" + addr})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/restaurante_recibir_pedido", strings.NewReader(`{"idMenu":1}`)))
	if rec.Code != http.StatusOK || rec.Body.String() != `{"idMenu":1}` || llamadas.Load() != 1 {
		t.Fatalf("got %d %q after %d calls; want the request delivered exactly once", rec.Code, rec.Body, llamadas.Load())
	}
}

func TestDuranteElCalentamientoEsperaLaRuta(t *testing.T) {
	destinoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"message":"ok","id":1}`))
	}))
	defer destinoSrv.Close()

	h := router(nuevoRegistro(), func() bool { return false }, 2*time.Second)
	go func() { // the service heartbeats a moment after the ESB pod started
		time.Sleep(150 * time.Millisecond)
		registrar(t, h, ServicioData{Host: "8081", Nombre: "estado_pedido", Ruta: "/estado_pedido",
			Padre: "restaurante", Method: "GET", Url: destinoSrv.URL})
	}()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/restaurante_estado_restaurante", strings.NewReader(`{"id":1}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("during warm-up: %d %s, want the request to wait for the route", rec.Code, rec.Body)
	}
}

func TestDespuesDelCalentamientoFallaRapido(t *testing.T) {
	h := router(nuevoRegistro(), func() bool { return false }, 0)
	inicio := time.Now()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/restaurante_estado_restaurante", strings.NewReader(`{}`)))
	if rec.Code != http.StatusServiceUnavailable || time.Since(inicio) > 100*time.Millisecond {
		t.Fatalf("got %d after %v, want an immediate 503", rec.Code, time.Since(inicio))
	}
}
