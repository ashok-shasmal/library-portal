package main

import (
	"fmt"

	"github.com/ashok-shasmal/library-portal/library"
)

func main() {
	fmt.Println("#### Starting Library Portal Server ####")
	lib := library.Init()
	lib.InitDB()
	lib.InitServer()
}
