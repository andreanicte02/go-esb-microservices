package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/andreanicte02/go-esb-microservices/comun"
	"github.com/gorilla/mux"
)

// struct de un mensaje generico
type JSONMessageGeneric struct {
	Message string `json:"message"`
	Id      int    `json:"id"`
}

// ServicioData describes one operation a microservice registered with the bus.
// Url (added in 2026) is how to reach the service; registrations without it fall
// back to the 2021 port -> hostname mapping in getService.
type ServicioData struct {
	Host   string
	Nombre string
	Ruta   string
	Padre  string
	Method string
	Url    string `json:",omitempty"`
}

func getService(host string) string {
	if host == "8085" {
		return "esb"
	} else if host == "8080" {
		return "cliente"
	} else if host == "8081" {
		return "restaurante"
	} else if host == "8082" {
		return "repartidor"
	}
	return "8080"
}

// decodificador
func Decodificador(body io.ReadCloser, data *JSONMessageGeneric) JSONMessageGeneric {
	decoder := json.NewDecoder(body)
	decoder.Decode(data)
	return *data
}

// buscar en array
func GetDataService(array []ServicioData, padre string, nombreServicio string) (ServicioData, bool) {
	for i := 0; i < len(array); i++ {
		if array[i].Padre == padre && array[i].Nombre == nombreServicio {
			return array[i], true
		}
	}
	return ServicioData{}, false
}

func Suma(numero1, numero2 int) (resultado int) {
	resultado = numero1 + numero2
	return
}

// registro is the service registry. HTTP handlers run concurrently, so it is
// guarded by a mutex (the 2021 version appended to a global slice without one).
// Registering the same (padre, nombre) again replaces the entry, so services can
// re-register on every heartbeat without the list growing.
type registro struct {
	mu        sync.RWMutex
	servicios []ServicioData
	cambio    chan struct{} // closed and replaced whenever a new route is registered
}

func nuevoRegistro() *registro {
	return &registro{cambio: make(chan struct{})}
}

func (r *registro) guardar(s ServicioData) (nuevo bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.servicios {
		if r.servicios[i].Padre == s.Padre && r.servicios[i].Nombre == s.Nombre {
			r.servicios[i] = s
			return false
		}
	}
	r.servicios = append(r.servicios, s)
	close(r.cambio) // wake up requests waiting for a route (see esperar)
	r.cambio = make(chan struct{})
	return true
}

// esperar is buscar, but if the route is not registered yet it waits for it
// until ctx ends.
func (r *registro) esperar(ctx context.Context, padre, nombre string) (ServicioData, bool) {
	for {
		r.mu.RLock()
		s, ok := GetDataService(r.servicios, padre, nombre)
		cambio := r.cambio
		r.mu.RUnlock()
		if ok {
			return s, true
		}
		select {
		case <-cambio:
		case <-ctx.Done():
			return ServicioData{}, false
		}
	}
}

func (r *registro) buscar(padre, nombre string) (ServicioData, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return GetDataService(r.servicios, padre, nombre)
}

func (r *registro) lista() []ServicioData {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append(make([]ServicioData, 0, len(r.servicios)), r.servicios...)
}

func destino(s ServicioData) string {
	base := s.Url
	if base == "" {
		base = "http://" + getService(s.Host) + ":" + s.Host
	}
	return base + s.Ruta
}

// rutas are the bus endpoints and the registered operation each one forwards to.
var rutas = []struct {
	ruta, metodo, padre, nombre string
}{
	{"/cliente_solicitar_pedido", "POST", "cliente", "solicitar_pedido"},
	{"/cliente_estado_restaurante", "GET", "cliente", "get_estado_restaurante"},
	{"/cliente_estado_repartidor", "GET", "cliente", "get_estado_repartidor"},
	{"/restaurante_recibir_pedido", "POST", "restaurante", "recibir_pedido"},
	{"/restaurante_estado_restaurante", "GET", "restaurante", "estado_pedido"},
	{"/restaurante_pedido_listo", "POST", "restaurante", "avisar_pedido_listo"},
	{"/repartidor_recibir_pedidio", "POST", "repartidor", "recibir_pedidio"},
	{"/repartidor_estado", "GET", "repartidor", "informar_estado_cliente"},
	{"/repartidor_marcar_pedido", "POST", "repartidor", "marcar_pedido"},
}

var clienteHTTP = &http.Client{Timeout: 10 * time.Second}

func responderJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// reenviar forwards the request body as-is to the registered operation and
// relays its status and body. In 2021 each endpoint decoded the body into a
// fixed struct and re-encoded it, which silently dropped fields (marcar_pedido
// never received idPedido), and a failed call crashed on a nil response.
//
// A new ESB pod starts with an empty registry and learns routes as services
// heartbeat. During its first `calentamiento`, a request for a route it does not
// know yet waits for that route to be registered instead of failing with 503.
func reenviar(reg *registro, padre, nombre string, calentamientoHasta time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := reg.buscar(padre, nombre)
		if !ok && time.Now().Before(calentamientoHasta) {
			ctx, cancel := context.WithDeadline(r.Context(), calentamientoHasta)
			s, ok = reg.esperar(ctx, padre, nombre)
			cancel()
		}
		if !ok {
			responderJSON(w, http.StatusServiceUnavailable,
				JSONMessageGeneric{Message: "servicio no registrado: " + padre + "/" + nombre, Id: -1})
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			responderJSON(w, http.StatusBadRequest, JSONMessageGeneric{Message: err.Error(), Id: -1})
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), s.Method, destino(s), bytes.NewReader(body))
		if err != nil {
			responderJSON(w, http.StatusInternalServerError, JSONMessageGeneric{Message: err.Error(), Id: -1})
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := enviar(req, body)
		if err != nil {
			log.Printf("reenviar %s/%s: %v", padre, nombre, err)
			responderJSON(w, http.StatusBadGateway,
				JSONMessageGeneric{Message: "no se pudo contactar a " + padre + ": " + err.Error(), Id: -1})
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// enviar sends the request, retrying only when the connection was refused.
// A refused connection means the request never reached the service, so it is
// safe to retry even a POST. It happens in Kubernetes for a moment after a pod
// becomes ready, until kube-proxy routes its Service to it.
func enviar(req *http.Request, body []byte) (*http.Response, error) {
	espera := 100 * time.Millisecond
	for intento := 1; ; intento++ {
		resp, err := clienteHTTP.Do(req)
		if err == nil || !errors.Is(err, syscall.ECONNREFUSED) || intento == 5 {
			return resp, err
		}
		select {
		case <-req.Context().Done():
			return nil, err
		case <-time.After(espera):
		}
		espera *= 2
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
}

func router(reg *registro, drenando func() bool, calentamiento time.Duration) http.Handler {
	calentamientoHasta := time.Now().Add(calentamiento)
	r := mux.NewRouter()
	// Which ESB pod answered: lets tests (and people) see when a new pod takes over.
	pod, _ := os.Hostname()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-ESB-Pod", pod)
			next.ServeHTTP(w, req)
		})
	})
	r.HandleFunc("/registrar_microservicio", func(w http.ResponseWriter, req *http.Request) {
		var s ServicioData
		if err := json.NewDecoder(req.Body).Decode(&s); err != nil || s.Padre == "" || s.Nombre == "" {
			responderJSON(w, http.StatusBadRequest, JSONMessageGeneric{Message: "descriptor inválido", Id: -1})
			return
		}
		if reg.guardar(s) {
			log.Printf("Servicio registrado: %s microservicio: %s -> %s", s.Nombre, s.Padre, destino(s))
		}
		responderJSON(w, http.StatusOK, JSONMessageGeneric{Message: "Servicio Registrado", Id: 1})
	}).Methods("POST")
	r.HandleFunc("/servicios", func(w http.ResponseWriter, _ *http.Request) {
		responderJSON(w, http.StatusOK, reg.lista())
	}).Methods("GET")
	r.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok\n"))
	}).Methods("GET")
	r.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if drenando() {
			http.Error(w, "apagándose\n", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok\n"))
	}).Methods("GET")
	for _, rt := range rutas {
		r.HandleFunc(rt.ruta, reenviar(reg, rt.padre, rt.nombre, calentamientoHasta)).Methods(rt.metodo)
	}
	return r
}

func main() {
	log.Println("escuchando el puerto 8085")
	// Long enough for every service to heartbeat at least once (HEARTBEAT_SECONDS).
	calentamiento := 8 * time.Second
	if n, err := strconv.Atoi(comun.Env("WARMUP_SECONDS", "8")); err == nil && n >= 0 {
		calentamiento = time.Duration(n) * time.Second
	}
	var srv *comun.Servidor
	srv = comun.NuevoServidor(":8085", router(nuevoRegistro(), func() bool { return srv.Drenando() }, calentamiento))
	srv.Correr()
}
