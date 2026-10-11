package main

import (
	"context"
	"strconv"

	"encoding/json"
	"fmt"
	"github.com/andreanicte02/go-esb-microservices/comun"
	"github.com/gorilla/mux"
	"io"
	"net/http"
)

func Suma(numero1, numero2 int) (resultado int) {
	resultado = numero1 + numero2
	return
}

// struct de un json generico
type JSONGenerico struct {
	Id int `json:"id"`
}

// struct de un mensaje generico
type JSONMessageGeneric struct {
	Message string `json:"message"`
	Id      int    `json:"id"`
}

// json para enviar la data y registrar los servicios en el ESB
type JSONMessageServices struct {
	Name string
	Ruta string
	HOST int
}

// struct que nos va ayudar a simular los menus en memoria
type Menu struct {
	Id          int
	Descripcion string
}

// struct que nos va ayudar a simular los clientes en memoria
type Cliente struct {
	Id     int
	Nombre string
}

// struct que va ayudar a almacenar la informacion de pedidos en memoria del restaurante
type Pedido struct {
	IdMenu    int `json:"idMenu"`
	IdCliente int `json:"idCliente"`
	IdEstado  int `json:"IdEstado"` //0 pendiente 1 completado

}

// struct que va ayudar a almacenar la informacion de pedidos en memoria del repartidor
type PedidoRepartidor struct {
	IdMenu           int    `json:"idMenu"`
	IdCliente        int    `json:"idCliente"`
	IdEstado         int    `json:"IdEstado"` //0 pendiente 1 completado
	DescripcionMenu  string `json:"DescripcionMenu"`
	IdPedido         int    `json:"idPedido"`
	EstadoRepartidor int    `json:"estadoRepartidor ya tomo el pedido o no"`
}

type ServicioData struct {
	Host   string
	Nombre string
	Ruta   string
	Padre  string
	Method string
}

// decodificador
func Decodificador(body io.ReadCloser, data *JSONMessageGeneric) JSONMessageGeneric {
	decoder := json.NewDecoder(body)
	decoder.Decode(data)
	return *data
}

// esbURL is the bus address: the Kubernetes Service name, or the compose hostname.
var esbURL = comun.Env("ESB_URL", "http://esb:8085")

var estadoRepartidor int = 0 //-1 indica que no tiene pedido pendiente, 0 que esta en caminio y 1 que ya se entrego
// se isa una hash en el lado del reapartidor para llevarla en memoria
var hashPedido = make(map[int]*PedidoRepartidor)

/**
w -> indica el contenido de la respuesta
r -> indica el contenido de la solicitud
*/

// servicio que recibe el pedido, indica al reapartidor que tiene pedidos que entregar
// recibe una json con la estrucutra de PedidoRepartidor y actualiza el estado
func recibirPedido(w http.ResponseWriter, r *http.Request) {

	data := PedidoRepartidor{}

	m := JSONMessageGeneric{}
	w.Header().Set("Content-Type", "application/json")

	err := json.NewDecoder(r.Body).Decode(&data)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if estadoRepartidor == 0 {
		data.EstadoRepartidor = 1
	}
	hashPedido[data.IdPedido] = &data

	m.Message = "pedidio realizado"
	m.Id = data.IdPedido
	json.NewEncoder(w).Encode(m)
	fmt.Print("pedido recibida: ")
	fmt.Println(data)

	if estadoRepartidor == 1 {
		return
	}
	estadoRepartidor = 1
}

// Recibe un json generico, con el id del pedido
// Rregresa el estado del pedido
func informarEstadoCliente(w http.ResponseWriter, r *http.Request) {

	data := JSONGenerico{}

	m := JSONMessageGeneric{}
	w.Header().Set("Content-Type", "application/json")

	err := json.NewDecoder(r.Body).Decode(&data)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pedido, existePedido := hashPedido[data.Id]

	if !existePedido {
		mensaje_error, _ := json.Marshal(JSONMessageGeneric{"El repartidor sigue en espera del pedido", -1})
		http.Error(w, string(mensaje_error), http.StatusBadRequest)
		fmt.Print("pedido recibida: ")
		fmt.Println(data)
		return
	}

	if pedido.EstadoRepartidor == -1 {
		m.Message = " el pedido ya fue entreagado "
	} else if estadoRepartidor == 1 && pedido.EstadoRepartidor == 0 {
		m.Message = "el reapartidor esta ocupado pero no con tu pedido"

	} else if estadoRepartidor == 1 && pedido.EstadoRepartidor == 1 {
		m.Message = "el repartidor va en camino con tu pedido"
	} else if estadoRepartidor == 0 {
		m.Message = "el reaprtidor no esta ocupado con ningun pedido"
	} else {
		m.Message = "tu pedido ya fue entregado"
	}

	m.Id = data.Id
	json.NewEncoder(w).Encode(m)
	fmt.Print("pedido recibida: ")
	fmt.Println(data)

}

// Recie un struct de pedidoRepartidor, solo para obtener el id del pedido
// actualiza el estado del repartidor en relacion con el pedido
func marcarPedido(w http.ResponseWriter, r *http.Request) {

	data := PedidoRepartidor{}
	m := JSONMessageGeneric{}
	w.Header().Set("Content-Type", "application/json")

	err := json.NewDecoder(r.Body).Decode(&data)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pedido, existePedido := hashPedido[data.IdPedido]

	if !existePedido {
		mensaje_error, _ := json.Marshal(JSONMessageGeneric{"Ese pedido no existe", -1})
		http.Error(w, string(mensaje_error), http.StatusBadRequest)
		fmt.Print("pedido recibida: ")
		fmt.Println(data)
		return
	}

	estadoRepartidor = 0
	pedido.EstadoRepartidor = -1 //pedido entregado

	m.Id = data.IdPedido
	m.Message = "id pedido: " + strconv.Itoa(m.Id) + " pedido completado"
	json.NewEncoder(w).Encode(m)
	fmt.Print("pedido recibida: ")
	fmt.Println(data)

}

// funcion que expone los servicios del repartidor
func handle(registro *comun.Registro) {

	router := mux.NewRouter()
	router.HandleFunc("/healthz", comun.Vivo).Methods("GET")
	router.HandleFunc("/readyz", registro.ListoHandler).Methods("GET")
	router.HandleFunc("/recibir_pedidio", recibirPedido).Methods("POST")
	router.HandleFunc("/informar_estado_cliente", informarEstadoCliente).Methods("GET")
	router.HandleFunc("/marcar_pedido", marcarPedido).Methods("POST")
	srv := comun.NuevoServidor(":8082", router)
	registro.Drenando = srv.Drenando
	srv.Correr()

}

func main() {
	// Registers in the background and keeps re-registering, so the service
	// survives the ESB starting late or restarting (see comun/registro.go).
	registro := comun.NuevoRegistro("repartidor", "8082",
		comun.Operacion{Nombre: "recibir_pedidio", Ruta: "/recibir_pedidio", Method: "POST"},
		comun.Operacion{Nombre: "informar_estado_cliente", Ruta: "/informar_estado_cliente", Method: "GET"},
		comun.Operacion{Nombre: "marcar_pedido", Ruta: "/marcar_pedido", Method: "POST"},
	)
	registro.Iniciar(context.Background())

	println("escuchando el puerto 8082")
	Suma(1, 2)
	handle(registro)

}
