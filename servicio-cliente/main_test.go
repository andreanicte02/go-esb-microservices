package main

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
)

func TestSuma(t *testing.T) {
	fmt.Println("Test funcion suma:")
	valor := Suma(7, 23)
	if valor != 30 {
		t.Error("Se esperaba 30 y se obtuvo", valor)
	}
}


/*
	preuba que verifica si la funcion para decodifcar funciona de manera cocrrecta
*/

func TestDecodificador(t *testing.T) {

	body := ioutil.NopCloser(strings.NewReader("\"{\"Message\":\"Hola mundo\",\"Id\":1}\""))
	message:= JSONMessageGeneric{"test",1}
	valor:= Decodificador(body,&message)

	if(valor.Message!="test"){
		t.Error("Se esperaba la palabra test y se obutvo", valor.Message)
	}
}