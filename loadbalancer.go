package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	//"github.com/gorilla/mux"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/openzipkin/zipkin-go"
	"github.com/openzipkin/zipkin-go/model"
	reporterhttp "github.com/openzipkin/zipkin-go/reporter/http"

	//"github.com/gorilla/mux"
	zipkinhttp "github.com/openzipkin/zipkin-go/middleware/http"
) //

// Server predstavlja informacije o pojedinom serveru
type Server struct {
	URL      string
	Name     string
	Weight   int
	Current  int
	MaxConns int
}

type MockServer struct {
	URL      string
	Name     string
	Weight   int
	Current  int
	MaxConns int
}

// LoadBalancer je struktura koja predstavlja load balancer
type LoadBalancer struct {
	servers       []*Server
	mutex         sync.Mutex
	activeServers map[string]bool
	mockServers   []*MockServer
}

type AdditionalInfo struct {
	User       string `json:"user"`
	AppVersion string `json:"app_version"`
}

// AddServer dodaje novi server u load balancer
func (lb *LoadBalancer) AddServer(name, url string, weight, maxConns int) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	if lb.activeServers == nil {
		lb.activeServers = make(map[string]bool)
	}

	lb.servers = append(lb.servers, &Server{
		URL:      url,
		Name:     name,
		Weight:   weight,
		Current:  0,
		MaxConns: maxConns,
	})
	lb.activeServers[url] = true
}

// Uklanjanje server iz load balancera
func (lb *LoadBalancer) RemoveServer(url string) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	for i, server := range lb.servers {
		if server.URL == url {
			lb.servers = append(lb.servers[:i], lb.servers[i+1:]...)
			break
		}
	}
	delete(lb.activeServers, url)
}

// Periodička provjera stanja servera
func (lb *LoadBalancer) HealthCheckPeriodically() {
	for {
		lb.mutex.Lock()
		for _, server := range lb.servers {
			resp, err := http.Get(server.URL)
			log.Printf("server.URL: %s ", server.URL)
			if err != nil {
				log.Printf("Server %s is down\n", server.Name)
				lb.activeServers[server.URL] = false
			} else {
				log.Printf("Server %s is up\n", server.Name)
				lb.activeServers[server.URL] = true
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
		lb.mutex.Unlock()

		time.Sleep(5 * time.Second)
	}
}

// Odabir servera prema weighted round-robin algoritmu
func (lb *LoadBalancer) chooseServerWeightedRoundRobin() *Server {
	var selected *Server
	totalWeight := 0

	for _, server := range lb.servers {
		if lb.activeServers[server.URL] {
			totalWeight += server.Weight
			server.Current += server.Weight

			if selected == nil || server.Current > selected.Current {
				selected = server
			}
		}
	}

	if selected != nil {
		selected.Current -= totalWeight
		if selected.Current < 0 {
			selected.Current = 0
		}
	}

	return selected
}

var (
	// Statistika zahtjeva od svih servera
	requestsByRoute = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "server_requests_by_route_total",
			Help: "Total number of requests by route",
		},
		[]string{"server_name", "server_url", "route"},
	)
	// Količina korištene memorije servera
	memoryUsageGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "server_memory_usage_bytes",
			Help: "Memory usage of the server",
		},
		[]string{"server_name", "server_url"},
	)
)

var (
	// Registrirani prometheus Registerer
	registerer prometheus.Registerer = prometheus.DefaultRegisterer
)

// Broj zahtjeva jednog servera
var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "server_requests_total",
			Help: "Total number of requests received by each server",
		},
		[]string{"server_name", "server_url"},
	)
)

// Ukupni statusni kodovi odgovora
var (
	responseStatusCodes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "server_response_status_codes_total",
			Help: "Total number of requests by response status codes for each server",
		},
		[]string{"server_name", "server_url", "status_code"},
	)
)

// Histogram za vremensko odvijanje zahtjeva na serveru
var (
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests",
			Buckets: []float64{0.0001, 0.1, 0.5, 1, 2, 5},
		},
		[]string{"URL", "method", "status"},
	)
)

var (
	// Broj trenutnih veza po serveru
	currentConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "server_current_connections",
			Help: "Current number of connections per server",
		},
		[]string{"server_name", "server_url"},
	)
)

// getMemoryUsageFromServer dohvaća memorijsku potrošnju servera
func getMemoryUsageFromServer(serverURL string) (uint64, error) {
	resp, err := http.Get(serverURL + "/memory")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("failed to get memory usage from server: status %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Alloc:") {
			valueStr := strings.TrimPrefix(line, "Alloc:")
			valueStr = strings.TrimSpace(valueStr)
			valueStr = strings.TrimSuffix(valueStr, " bytes") // Ukloni " bytes" s kraja stringa
			memAlloc, err := strconv.ParseUint(valueStr, 10, 64)
			if err != nil {
				return 0, err
			}
			return memAlloc, nil
		}
	}

	return 0, fmt.Errorf("unexpected response format from server")
}

//Vremenska provjera konekcija
func (lb *LoadBalancer) UpdateCurrentConnections(server *Server, count float64) {
	currentConnections.With(prometheus.Labels{
		"server_name": server.Name,
		"server_url":  server.URL,
	}).Set(count)
}

const endpointURL = "http://192.168.1.8:9411/api/v2/spans"

func newTracer(endpointURL string) (*zipkin.Tracer, error) {
	reporter := reporterhttp.NewReporter(endpointURL)

	localEndpoint := &model.Endpoint{ServiceName: "load-balancer", Port: 8000}

	sampler, err := zipkin.NewCountingSampler(1)
	if err != nil {
		return nil, err
	}

	// Stvoranje Zipkin tracera s reporterom i samplerom
	tracer, err := zipkin.NewTracer(reporter, zipkin.WithLocalEndpoint(localEndpoint), zipkin.WithSampler(sampler))
	if err != nil {
		return nil, err
	}

	return tracer, nil
}
func (lb *LoadBalancer) sendRequestToServer(url string, r *http.Request, tracer *zipkin.Tracer, outgoingSpan zipkin.Span) (*http.Response, error) {
	// Stvaranje zahtjeva prema serveru
	req, err := http.NewRequest(r.Method, url, r.Body)
	if err != nil {
		return nil, err
	}

	// Stvaranje spana za odlazni poziv
	outgoingCallSpan := tracer.StartSpan("outgoing-call", zipkin.Parent(outgoingSpan.Context()))
	defer outgoingCallSpan.Finish()

	// Postavljanje trace konteksta na zahtjev
	ctx := zipkin.NewContext(req.Context(), outgoingCallSpan)
	req = req.WithContext(ctx)

	// Slanje zahtjeva serveru
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// handleRequest rukuje dolaznim zahtjevima
func (lb *LoadBalancer) handleRequest(w http.ResponseWriter, r *http.Request, tracer *zipkin.Tracer, incomingSpan, outgoingSpan zipkin.Span) {

	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	if len(lb.servers) == 0 {
		http.Error(w, "No servers available", http.StatusServiceUnavailable)
		return
	}

	// Odabir aktivnog servera
	selected := lb.chooseServerWeightedRoundRobin()
	if selected == nil {
		http.Error(w, "No active servers available", http.StatusServiceUnavailable)
		return
	}

	// Dodatne informacije
	info := AdditionalInfo{
		User:       "Jan Tudan",
		AppVersion: "1.18",
	}

	incomingSpan.Tag("request_type", r.Method)

	for _, server := range lb.servers {
		// Stvaranje spana za svaki odlazni poziv prema serveru
		outgoingSpan := tracer.StartSpan(server.Name, zipkin.Parent(incomingSpan.Context()))
		defer outgoingSpan.Finish()

		// Poziv prema serveru
		resp, err := lb.sendRequestToServer(server.URL, r, tracer, outgoingSpan)

		// Dodavanje informacija na svaki server kao anotacija u spanove
		outgoingSpan.Tag("user", info.User)
		outgoingSpan.Tag("app_version", info.AppVersion)

		if err != nil {
			// Obrada greške prilikom slanja zahtjeva
			log.Fatalf("Failed to send request to %s: %v", server.Name, err)
		}
		defer resp.Body.Close()
	}
	// Ažuriranje broja trenutnih veza za odabrani server
	lb.UpdateCurrentConnections(selected, float64(selected.Current))

	// Promatranje zahtjeva po određenoj ruti servera
	route := r.URL.EscapedPath()
	requestsByRoute.With(prometheus.Labels{"server_name": selected.Name, "server_url": selected.URL, "route": route}).Inc()

	statusCode := 200
	// Prikaz zahtjeva po svim serverima: server_response_status_codes_total
	// Komanda za broj ukupnih statusnih kodova odgovora: server_response_status_codes_total{server_name="Server 2", server_url="http://192.168.1.8:8002"}
	// Za zbroj svih servera: sum by (status_code) (server_response_status_codes_total)
	responseStatusCodes.With(prometheus.Labels{
		"server_name": selected.Name,
		"server_url":  selected.URL,
		"status_code": strconv.Itoa(statusCode),
	}).Inc()

	// Provjera zahtjeva za određeni server: server_requests_total{server_name="Server 3", server_url="http://192.168.1.8:8003"}
	requestsTotal.With(prometheus.Labels{"server_name": selected.Name, "server_url": selected.URL}).Inc()

	proxyURL, err := url.Parse(selected.URL)
	if err != nil {
		log.Printf("Failed to parse target URL: %s\n", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	// Dohvat memorijske potrošnje iz servera
	memUsage, err := getMemoryUsageFromServer(selected.URL)
	if err != nil && !strings.Contains(strings.ToLower(selected.Name), "mock") {
		if err.Error() != "mock server" {
			log.Printf("Failed to get memory usage from server: %s\n", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		// Prikaz potrošenosti memorije za server u metrikama/Prometheusu
		if !strings.Contains(strings.ToLower(selected.Name), "mock") {
			{
				memoryUsageGauge.With(prometheus.Labels{"server_name": selected.Name, "server_url": selected.URL}).Set(float64(memUsage))
			}
		}
	}

	start := time.Now()

	// Logika rukovanja zahtjevom

	duration := time.Since(start).Seconds()
	method := r.Method
	status := strconv.Itoa(statusCode)

	requestDuration.With(prometheus.Labels{
		"URL":    selected.URL,
		"method": method,
		"status": status,
	}).Observe(duration)

	proxy := httputil.NewSingleHostReverseProxy(proxyURL)
	proxy.ServeHTTP(w, r)

}

// GetServerByName vraća server na temelju imena
func (lb *LoadBalancer) GetServerByName(name string) *Server {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	for _, server := range lb.servers {
		if server.Name == name {
			return server
		}
	}

	return nil
}

// GetServers vraća sve servere u load balanceru
func (lb *LoadBalancer) GetServers() []*Server {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	return lb.servers
}

// DisableServer onemogućava određeni server u load balanceru
func (lb *LoadBalancer) DisableServer(name string) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	for _, server := range lb.servers {
		if server.Name == name {
			lb.activeServers[server.URL] = false
			break
		}
	}
}

// Omogućavanje određenog servera u load balanceru
func (lb *LoadBalancer) EnableServer(name string) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	for _, server := range lb.servers {
		if server.Name == name {
			lb.activeServers[server.URL] = true
			break
		}
	}
}

// Biranje nasumičnog servera iz load balancera
func (lb *LoadBalancer) RandomServer() *Server {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	rand.Seed(time.Now().UnixNano())
	randomIndex := rand.Intn(len(lb.servers))

	return lb.servers[randomIndex]
}

// Uklanjanje svih servera iz load balancera
func (lb *LoadBalancer) ClearServers() {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	lb.servers = []*Server{}
	lb.activeServers = make(map[string]bool)
}

func main() {
	lb := LoadBalancer{
		activeServers: make(map[string]bool),
	}

	mux := http.NewServeMux()

	// Lista servera u load balanceru
	lb.AddServer("Server 1", "http://192.168.1.8:8001", 2, 100)
	lb.AddServer("Server 2", "http://192.168.1.8:8002", 4, 100)
	lb.AddServer("Server 3", "http://192.168.1.8:8003", 6, 100)
	lb.AddServer("Mock server 1", "http://192.168.1.8:8004", 1, 100)
	lb.AddServer("Mock server 2", "http://192.168.1.8:8005", 3, 100)
	lb.AddServer("Mock server 3", "http://192.168.1.8:8006", 5, 100)

	prometheus.MustRegister(requestsByRoute)
	prometheus.MustRegister(memoryUsageGauge)
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(currentConnections)

	tracer, err := newTracer(endpointURL)
	if err != nil {
		log.Fatalf("Failed to create Zipkin tracer: %v", err)
	}

	// Generiranje Trace ID-a
	span := tracer.StartSpan("my-operation")
	defer span.Finish()

	// Dohvati Trace ID
	traceID := span.Context().TraceID.String()
	log.Printf("Trace ID: %s\n", traceID)

	handler := zipkinhttp.NewServerMiddleware(
		tracer,
		zipkinhttp.SpanName("root-handler"),
		zipkinhttp.TagResponseSize(true),
	)(mux)

	// Postavljanje HTTP servera
	server := &http.Server{
		Addr:    ":8000",
		Handler: promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handler),
	}

	go func() {
		// Prikaz svih metrika iz prometheusa
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":9090", nil)
	}()

	// Periodička provjera stanja servera
	go lb.HealthCheckPeriodically()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Stvaranje spana za dolazni zahtjev

		incomingSpan := tracer.StartSpan("incoming-request")
		defer incomingSpan.Finish()

		// Odlazni poziv prema drugom servisu s roditeljskim spanom "incoming-request"
		outgoingSpan := tracer.StartSpan("outgoing-call", zipkin.Parent(incomingSpan.Context()))
		defer outgoingSpan.Finish()

		// Obrada dolaznog zahtjeva
		lb.handleRequest(w, r, tracer, incomingSpan, outgoingSpan) // Poziv originalnog handlera
	})

	mux.Handle("/metrics", promhttp.Handler())

	// Ruta za informacije o određenom serveru
	// Traženje po serveru: http://192.168.1.5:8000/serverName?name=Server%203
	mux.HandleFunc("/serverName", func(w http.ResponseWriter, r *http.Request) {
		serverName := r.URL.Query().Get("name")
		if serverName != "" {
			server := lb.GetServerByName(serverName)
			if server != nil {
				fmt.Fprintf(w, "Server: %s, URL: %s, Weight: %d, Current: %d, MaxConns: %d\n",
					server.Name, server.URL, server.Weight, server.Current, server.MaxConns)
			} else {
				http.Error(w, "Server not found", http.StatusNotFound)
			}
		} else {
			http.Error(w, "Missing server name parameter", http.StatusBadRequest)
		}
	})

	// Memorija za zaseban server
	mux.HandleFunc("/memory", func(w http.ResponseWriter, r *http.Request) {
		server := lb.RandomServer()

		for server != nil && strings.Contains(strings.ToLower(server.Name), "mock") {
			// Ako je server mock server generira se novi random server
			server = lb.RandomServer()
		}
		if server != nil {
			proxyURL, err := url.Parse(server.URL)
			if err != nil {
				log.Printf("Failed to parse target URL: %s\n", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			proxy := httputil.NewSingleHostReverseProxy(proxyURL)
			proxy.ServeHTTP(w, r)
		} else {
			http.Error(w, "No servers available", http.StatusServiceUnavailable)
		}
	})

	// Prikaz svih servera
	mux.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		servers := lb.GetServers()
		for _, server := range servers {
			fmt.Fprintf(w, "Name: %s, URL: %s\n", server.Name, server.URL)
		}
	})

	// Onemogući server
	mux.HandleFunc("/disable", func(w http.ResponseWriter, r *http.Request) {
		serverName := r.URL.Query().Get("name")
		if serverName != "" {
			lb.DisableServer(serverName)
			fmt.Fprintf(w, "Server '%s' disabled.\n", serverName)
		} else {
			http.Error(w, "Missing server name parameter", http.StatusBadRequest)
		}
	})

	// Omogući server
	mux.HandleFunc("/enable", func(w http.ResponseWriter, r *http.Request) {
		serverName := r.URL.Query().Get("name")
		if serverName != "" {
			lb.EnableServer(serverName)
			fmt.Fprintf(w, "Server '%s' enabled.\n", serverName)
		} else {
			http.Error(w, "Missing server name parameter", http.StatusBadRequest)
		}
	})

	// Nasumični server
	mux.HandleFunc("/random", func(w http.ResponseWriter, r *http.Request) {
		randomServer := lb.RandomServer()
		if randomServer != nil {
			fmt.Fprintf(w, "Random server: %s\n", randomServer.Name)
		} else {
			http.Error(w, "No servers available", http.StatusServiceUnavailable)
		}
	})

	// Ukloni sve servere
	mux.HandleFunc("/clear", func(w http.ResponseWriter, r *http.Request) {
		lb.ClearServers()
		fmt.Fprintln(w, "All servers removed.")
	})

	fmt.Println("Load balancer started on port 8000")

	log.Fatal(server.ListenAndServe())
}
