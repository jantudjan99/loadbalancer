package main

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
)

func server3Handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to server 3")
}

func memoryUsageHandler(w http.ResponseWriter, r *http.Request) {
	stats := runtime.MemStats{}
	runtime.ReadMemStats(&stats)

	fmt.Fprintf(w, "Alloc: %d bytes\n", stats.Alloc)
	fmt.Fprintf(w, "TotalAlloc: %d bytes\n", stats.TotalAlloc)
	fmt.Fprintf(w, "Sys: %d bytes\n", stats.Sys)
	fmt.Fprintf(w, "Mallocs: %d\n", stats.Mallocs)
	fmt.Fprintf(w, "Frees: %d\n", stats.Frees)
}

func main() {

	http.HandleFunc("/", server3Handler)
	http.HandleFunc("/memory", memoryUsageHandler)

	log.Println("Server 3 is running and change...")
	log.Fatal(http.ListenAndServe(":8003", nil))
}
