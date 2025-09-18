package main

import (
	"fmt"
	config "load-balancer/Config"
	"load-balancer/proxy"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func recordMetrics() {
	go func() {
		for {
			opsProcessed.Inc()
			time.Sleep(2 * time.Second)
		}
	}()
}

var (
	opsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "myapp_processed_ops_total",
		Help: "The total number of processed events",
	})
)

func main() {
	recordMetrics()
	cfg, err := config.LoadConfig("config.yaml")

	server := proxy.NewProxyServer(cfg)

	if err != nil {
		fmt.Println("file error: %w", err)
		return
	}

	server.StartHealthMonitor(5 * time.Second)
	go server.WatchConfig("config.yaml")

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", server.ProxyHandler)

	log.Println("Reverse proxy running on :9090")
	err = http.ListenAndServe(":9090", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
