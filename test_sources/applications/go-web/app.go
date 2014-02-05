package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	app_dir := os.Args[0]
	fmt.Println("Serving files from %v", app_dir)
	log.Fatal(http.ListenAndServe(":8080", http.FileServer(http.Dir(app_dir))))
}
