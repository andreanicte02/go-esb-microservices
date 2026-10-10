package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"log"
	"math/rand"
	"net/http"
)

func Suma(numero1, numero2 int) (resultado int) {
	resultado = numero1 + numero2
	return
}


//struct de un json generico
type JSONGenerico struct {

	Id int `json:"id"`

}
//struct de un mensaje generico
type JSONMessageGeneric struct {

	Message string `json:"message"`
	Id      int `json:"id"`

}

//json para enviar la data y registrar los servicios en el ESB
type JSONMessageServices struct {

	Name string
	Ruta string
	HOST  int

}


//struct que nos va ayudar a simular los menus en memoria
type Menu struct {
	Id int
	Descripcion string
}

//struct que nos va ayudar a simular los clientes en memoria
type Cliente struct {
	Id int
	Nombre string
}

//struct que va ayudar a almacenar la informacion de pedidos en memoria del restaurante
type Pedido struct {

	IdMenu int `json:"idMenu"`
	IdCliente int `json:"idCliente"`
	IdEstado int `json:"IdEstado"` //0 pendiente 1 completado


}


//struct que va ayudar a almacenar la informacion de pedidos en memoria del repartidor
type PedidoRepartidor struct {

	IdMenu int `json:"idMenu"`
	IdCliente int `json:"idCliente"`
	IdEstado int `json:"IdEstado"` //0 pendiente 1 completado
	DescripcionMenu string `json:"DescripcionMenu"`
	IdPedido int  `json:"idPedido"`
	EstadoRepartidor int `json:"estadoRepartidor ya tomo el pedido o no"`


}

func LogSalida(data JSONGenerico, m JSONMessageGeneric)  {
	fmt.Println("data recibida: ")
	fmt.Println(data)
	fmt.Println("respuesta de la solicitud de pedido:")
	fmt.Println(m)
}


type ServicioData struct {

	Host string
	Nombre string
	Ruta string
	Padre string
	Method string


}

//decodificador
func Decodificador(body io.ReadCloser, data *JSONMessageGeneric) JSONMessageGeneric  {
	decoder:= json.NewDecoder(body)
	decoder.Decode(data)
	return *data
}


//funcion para registrar servicios
func RegistrarServicio(servicio *ServicioData, method string, host string, nameSerivce string){

	dataRequest,_:= json.Marshal(servicio)
	req,err := http.NewRequest(method, "http://esb:"+host+ nameSerivce, bytes.NewBuffer(dataRequest))
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error on response.\n[ERRO] -", err)
	}

	defer resp.Body.Close()
	var data = Decodificador(resp.Body,&JSONMessageGeneric{"",0})
	fmt.Println("info. recibida")
	fmt.Println(data)

}



var HashPedido = make(map[int]int)
var Codigo  = -1
var IdPedido = -1
/*
	req,err :=  http.NewRequest(1, 2, 3)
		primer argumento, se indica el metodo a utilizar POST | GET | PUT etc
		segundo argumetno, se indica el url de la api a consumir
		tercer agumento, se indica el contenido del pody de la peticion
*/

/*
	agregar informacion al header de una peticion
	req.Header.Add(1, 2)
		primer argumento indica el tipo que se va agregar
		segundo argument indica el valor
*/


//funcion para solicitar pedido
func PeticionSolicitarPedido(pedido *Pedido) JSONMessageGeneric{

	dataRequest,_:= json.Marshal(pedido)
	req,err := http.NewRequest("POST", "http://esb:8085/restaurante_recibir_pedido", bytes.NewBuffer(dataRequest))
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error on response.\n[ERRO] -", err)
	}

	defer resp.Body.Close()

	var data = Decodificador(resp.Body,&JSONMessageGeneric{"",0})

	HashPedido[data.Id]=data.Id
	IdPedido = data.Id
	return data


}

//funcion para solicitar pedido al restaurante
func PeticionSolicitarEstadoRestaurante(jsonGeneric *JSONGenerico) JSONMessageGeneric{

	dataRequest,_:= json.Marshal(jsonGeneric)
	req,err := http.NewRequest("GET", "http://esb:8085/restaurante_estado_restaurante", bytes.NewBuffer(dataRequest))
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error on response.\n[ERRO] -", err)
	}

	defer resp.Body.Close()

	var data = Decodificador(resp.Body,&JSONMessageGeneric{"",0})

	HashPedido[data.Id]=data.Id


	return data



}

//funcion para solicitar pedido al repartidor
func PeticionEstadoRepartidor(jsonGeneric *JSONGenerico) JSONMessageGeneric{

	dataRequest,_:= json.Marshal(jsonGeneric)
	req,err := http.NewRequest("GET", "http://esb:8085/repartidor_estado", bytes.NewBuffer(dataRequest))
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Error on response.\n[ERRO] -", err)
	}

	defer resp.Body.Close()


	var data = Decodificador(resp.Body,&JSONMessageGeneric{"",0})

	HashPedido[data.Id]=data.Id
	return data

}


func menuRandom()  int {

	return rand.Intn(2 - 0) + 0
}


func solicitarPedido(w http.ResponseWriter, r *http.Request)  {


	data:= JSONGenerico{}

	m:= JSONMessageGeneric{}
	w.Header().Set("Content-Type","application/json")

	err := json.NewDecoder(r.Body).Decode(&data)

	if err != nil{
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m=PeticionSolicitarPedido(&Pedido{menuRandom(),data.Id,0})
	IdPedido = m.Id
	json.NewEncoder(w).Encode(m)
	LogSalida(data,m)

}

func getEstadoRestaurante(w http.ResponseWriter, r *http.Request)  {


	data:= JSONGenerico{}

	m:= JSONMessageGeneric{}
	w.Header().Set("Content-Type","application/json")

	err := json.NewDecoder(r.Body).Decode(&data)

	if err != nil{
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m=PeticionSolicitarEstadoRestaurante(&data)
	json.NewEncoder(w).Encode(m)

	LogSalida(data,m)



}

func getEstadoRepartidor(w http.ResponseWriter, r *http.Request)  {


	data:= JSONGenerico{}

	m:= JSONMessageGeneric{}
	w.Header().Set("Content-Type","application/json")

	err := json.NewDecoder(r.Body).Decode(&data)

	if err != nil{
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m= PeticionEstadoRepartidor(&data)
	json.NewEncoder(w).Encode(m)
	LogSalida(data,m)




}


func handle() {

	router := mux.NewRouter()
	router.HandleFunc("/solicitar_pedido",solicitarPedido).Methods("POST")
	router.HandleFunc("/get_estado_restaurante",getEstadoRestaurante).Methods("GET")
	router.HandleFunc("/get_estado_repartidor",getEstadoRepartidor).Methods("GET")
	http.ListenAndServe(":8080", router)
}


func main()  {

	RegistrarServicio(&ServicioData{"8080", "solicitar_pedido","/solicitar_pedido","cliente","POST"}, "POST","8085","/registrar_microservicio")
	RegistrarServicio(&ServicioData{"8080", "get_estado_restaurante","/get_estado_restaurante","cliente","GET"}, "POST","8085","/registrar_microservicio")
	RegistrarServicio(&ServicioData{"8080", "get_estado_repartidor","/get_estado_repartidor","cliente","GET"}, "POST","8085","/registrar_microservicio")

	fmt.Println("Escuhando puerto 8080")
	HashPedido = make(map[int]int)
	Codigo  = -1
	IdPedido = -1
	Suma(1,1)
	handle()


}