package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"io/ioutil"
	"log"
	"net/http"
)



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

//se usan hash para llevar el control de clientes, pedido, menu en memoria
var hashCliente = make(map[int]*Cliente)
var hashPedido = make(map[int]*Pedido)
var hashMenu = make(map[int]*Menu)
//contador globla y unico que va llevar el id de los pedidos
var idPedido = 0

/**
w -> indica el contenido de la respuesta
r -> indica el contenido de la solicitud
*/

//funcion que recibe el pedido y lo guarda en memoria, recibe una estructura de pedido en el body
func recibirPedido(w http.ResponseWriter, r *http.Request)  {

	data:= Pedido{}
	m:= JSONMessageGeneric{}
	w.Header().Set("Content-Type","application/json")

	err := json.NewDecoder(r.Body).Decode(&data)

	if err != nil{
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_,existeCliente := hashCliente[data.IdCliente]
	_,existeMenu := hashMenu[data.IdMenu]

	if !existeCliente || !existeMenu{

		mensaje_error,_ := json.Marshal(JSONMessageGeneric{"El menu o el cliente no existen",-1})
		http.Error(w, string(mensaje_error), http.StatusBadRequest)
		return
	}


	//se retorna un json con el id del pedido
	hashPedido[idPedido]=&data
	m.Message =  "pedidio realizado"
	m.Id = idPedido
	json.NewEncoder(w).Encode(m)
	fmt.Print("data recibida: ")
	fmt.Println(data)
	idPedido ++
	fmt.Println("Pedido registrado en el servicio de restaurante")


}

////funcion que recibe el id del pedido y devuelve el estado del pedido
func estadoPedido(w http.ResponseWriter, r *http.Request)  {

	data:= JSONGenerico{}
	m:= JSONMessageGeneric{}
	w.Header().Set("Content-Type","application/json")
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil{
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pedido,existePedido := hashPedido[data.Id]

	if !existePedido{

		mensaje_error,_ := json.Marshal(JSONMessageGeneric{"Ese pedido no existe",-1})
		http.Error(w, string(mensaje_error), http.StatusBadRequest)
		return

	}

	var strStatus string

	if strStatus = "pendiente"; pedido.IdEstado == 1{

		strStatus = "completo"
	}


	m.Message = hashCliente[pedido.IdCliente].Nombre + "respuesta estado de pedidio " + strStatus
	m.Id = pedido.IdEstado
	json.NewEncoder(w).Encode(m)

	fmt.Print("data recibida: ")
	fmt.Println(data)

}

////funcion que recibe el id del pedido y indica en memoria y al repartidor que el pedido ya se puede recoger
func avisarPedidoListo(w http.ResponseWriter, r *http.Request)  {

	data:= JSONGenerico{}
	m:= JSONMessageGeneric{}
	w.Header().Set("Content-Type","application/json")
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil{
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pedido,existePedido := hashPedido[data.Id]

	if !existePedido{

		mensaje_error,_ := json.Marshal(JSONMessageGeneric{"Ese pedido no existe",-1})
		http.Error(w, string(mensaje_error), http.StatusBadRequest)
		return

	}

	simulacionEntregaPedidoAlRepartidor(data.Id)
	m.Message = hashCliente[pedido.IdCliente].Nombre + " pedido mas que listo "
	m.Id = 1
	pedido.IdEstado = 1
	json.NewEncoder(w).Encode(m)
	fmt.Print("data recibida: ")
	fmt.Println(data)


}
//fncion que simula la comunicacion entre el servicio del repartidor y el restaurante
func simulacionEntregaPedidoAlRepartidor(idPedido int){


	pedido,existePedido := hashPedido[idPedido]

	if !existePedido || pedido.IdEstado==1{
		return
	}

	data,_:= json.Marshal(PedidoRepartidor{pedido.IdMenu,pedido.IdCliente, pedido.IdEstado, hashMenu[pedido.IdMenu].Descripcion,idPedido,0})
	req,err := http.NewRequest("POST", "http://esb:8085/repartidor_recibir_pedidio", bytes.NewBuffer(data))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		log.Println("Error on response.\n[ERRO] -", err)
	}
	body, _ := ioutil.ReadAll(resp.Body)
	log.Println(string([]byte(body)))
	pedido.IdEstado = 1



}


//funcion que expone los servicios del restaurante
func handle()  {

	router := mux.NewRouter()
	router.HandleFunc("/recibir_pedido",recibirPedido).Methods("POST")
	router.HandleFunc("/estado_pedido",estadoPedido).Methods("GET")
	router.HandleFunc("/avisar_pedido_listo",avisarPedidoListo).Methods("POST")
	http.ListenAndServe(":8081", router)

}


func Suma(numero1, numero2 int) (resultado int) {
	resultado = numero1 + numero2
	return
}


func main()  {

	RegistrarServicio(&ServicioData{"8081", "recibir_pedido","/recibir_pedido","restaurante","POST"}, "POST","8085","/registrar_microservicio")
	RegistrarServicio(&ServicioData{"8081", "estado_pedido","/estado_pedido","restaurante","GET"}, "POST","8085","/registrar_microservicio")
	RegistrarServicio(&ServicioData{"8081", "avisar_pedido_listo","/avisar_pedido_listo","restaurante","POST"}, "POST","8085","/registrar_microservicio")


	hashMenu[0]=&Menu{0,"menu1"}
	hashMenu[1]=&Menu{1,"menu1"}
	hashMenu[2]=&Menu{2,"menu1"}

	hashCliente[0]=&Cliente{0,"cliente1"}
	hashCliente[1]=&Cliente{1,"cliente2"}
	hashCliente[2]=&Cliente{2,"cliente2"}

	println("escuchando el puerto 8081")
	Suma(1,2)
	handle()



}