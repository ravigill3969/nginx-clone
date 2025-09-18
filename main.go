package main

import (
	"fmt"
	config "load-balancer/Config"
	"load-balancer/proxy"
	"log"
	"net/http"
	"time"
)

func main() {

	cfg, err := config.LoadConfig("config.yaml")

	server := proxy.NewProxyServer(cfg)

	if err != nil {
		fmt.Println("file error: %w", err)
		return
	}

	server.StartHealthMonitor(5 * time.Second)

	http.HandleFunc("/", server.ProxyHandler)

	log.Println("Reverse proxy running on :9090")
	err = http.ListenAndServe(":9090", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
