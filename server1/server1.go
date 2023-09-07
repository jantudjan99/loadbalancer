package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

var startTime = time.Now()
var startCPUTime float64

func init() {
	startCPUTime = float64(time.Now().UnixNano())
}

func server1Handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome to server 1")
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

func cpuDetailsHandler(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("lscpu")
	output, err := cmd.Output()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cpuInfo := string(output)
	lines := strings.Split(cpuInfo, "\n")

	for _, line := range lines {
		fmt.Fprintln(w, line)
	}

	fmt.Println("------------------------------")
}

func cpuTimeHandler(w http.ResponseWriter, r *http.Request) {
	currentCPUTime := float64(time.Now().UnixNano())
	cpuUsage := (currentCPUTime - startCPUTime) / 1e9 // Pretvaranje iz nanosekundi u sekunde

	uptime := time.Since(startTime).Seconds()

	fmt.Fprintf(w, "Total CPU Usage: %.2f seconds\n", cpuUsage)
	fmt.Fprintf(w, "Uptime: %.2f seconds\n", uptime)
}

func residentMemoryHandler(w http.ResponseWriter, r *http.Request) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	residentMemory := stats.Sys - stats.HeapReleased

	fmt.Fprintf(w, "ResidentMemory: %d bytes\n", residentMemory)
}

func main() {
	http.HandleFunc("/", server1Handler)

	http.HandleFunc("/memory", memoryUsageHandler)

	http.HandleFunc("/resident-memory", residentMemoryHandler)

	http.HandleFunc("/cpu-time", cpuTimeHandler)

	http.HandleFunc("/cpu-details", cpuDetailsHandler)

	log.Println("Server 1 is running and change...")
	log.Fatal(http.ListenAndServe(":8001", nil))
}
