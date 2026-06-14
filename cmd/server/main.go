package main

import (
	"fmt"

	"github.com/ashok-shasmal/library-portal/library"
)

func main() {
	fmt.Println("#### Starting Library Portal Server ####")
	lib := library.Init()
	logFile := lib.InitLogger()
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()
	lib.InitDB()
	lib.InitServer()
}
