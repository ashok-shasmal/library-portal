package main

import (
	"fmt"

	"github.com/ashok-shasmal/library-portal/internal"
)

func main() {
	fmt.Println("#### Starting Library Portal Server ####")
	lib := internal.Init()
	lib.InitDB()
	lib.InitServer()
}
