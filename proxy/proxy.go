package proxy

import (
	"context"
	"fmt"
	config "load-balancer/Config"
	"load-balancer/utils"
	"log"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

type ProxyServer struct {
	cfg               *config.Config
	activeConnections map[string]*int32
	Status            map[string]int
	counter           uint32
}

func NewProxyServer(cfg *config.Config) *ProxyServer {
	activeConnections := make(map[string]*int32)
	status := make(map[string]int)

	for _, b := range cfg.Backends {
		var zero int32
		activeConnections[b.URL] = &zero
		status[b.URL] = 0

	}

	return &ProxyServer{
		cfg:               cfg,
		activeConnections: activeConnections,
		Status:            status,
	}
}

func (p *ProxyServer) StartHealthMonitor(interval time.Duration) {
	go func() {
		for _, currentServer := range p.cfg.Backends {
			url := utils.BackendURL(fmt.Sprintf("%s%s", currentServer.URL, p.cfg.HealthCheckPath))
			if err := url.Validate(); err != nil {
				p.Status[currentServer.URL] = 1
			} else {
				p.Status[currentServer.URL] = 0
			}
		}
		time.Sleep(interval)

	}()
}

func (p *ProxyServer) GetHealthyBackends() []config.Backend {
	var healthy []config.Backend
	for _, b := range p.cfg.Backends {
		if p.Status[b.URL] == 0 { // 0 = healthy
			healthy = append(healthy, config.Backend{URL: b.URL, Weight: b.Weight})
		}
	}
	return healthy
}

func (p *ProxyServer) GetRandomBackend() string {
	healthy := p.GetHealthyBackends()
	idx := rand.IntN(len(healthy))
	return healthy[idx].URL
}

func (p *ProxyServer) GetLeastConnBackend() (string, error) {
	var chosen string
	var minConns int32 = math.MaxInt32

	healthy := p.GetHealthyBackends()

	for _, current := range healthy {
		count := p.activeConnections[current.URL]

		healthURL := fmt.Sprintf("%s%s", current.URL, p.cfg.HealthCheckPath)

		if err := utils.BackendURL(healthURL).Validate(); err != nil {
			continue
		}

		if *count < minConns {
			minConns = *count
			chosen = current.URL
		}
	}

	if chosen == "" {
		return "", fmt.Errorf("no healthy backend available")
	}

	atomic.AddInt32(p.activeConnections[chosen], 1)

	return chosen, nil
}

func (p *ProxyServer) GetRoundRobinBackend() string {
	healthy := p.GetHealthyBackends()
	idx := atomic.AddUint32(&p.counter, 1)
	return healthy[idx%uint32(len(healthy))].URL
}

func (p *ProxyServer) GetWeightedRoundRobinBackend() (string, error) {
	backends := p.GetHealthyBackends()
	var expanded []string
	for _, b := range backends {
		if b.Weight <= 0 {
			continue
		}
		for i := 0; i < b.Weight; i++ {
			expanded = append(expanded, b.URL)
		}
	}

	if len(expanded) == 0 {
		return "", fmt.Errorf("no healthy backends with valid weights")
	}

	idx := atomic.AddUint32(&p.counter, 1)
	return expanded[idx%uint32(len(expanded))], nil
}

func (p *ProxyServer) GetNextBackend() (string, error) {

	switch p.cfg.Strategy {
	case "leastconn":
		return p.GetLeastConnBackend()
	case "weighted":
		return p.GetWeightedRoundRobinBackend()
	case "random":
		return p.GetRandomBackend(), nil
	default:
		return p.GetRoundRobinBackend(), nil
	}
}

func (p *ProxyServer) ProxyHandler(w http.ResponseWriter, r *http.Request) {
	if p.cfg.ErrorRate > 0 && rand.Float64() < p.cfg.ErrorRate {
		http.Error(w, "Simulated error (chaos)", http.StatusInternalServerError)
		log.Printf("[CHAOS] Injected error for %s %s", r.Method, r.URL.Path)
		return
	}

	start := time.Now()
	tried := make(map[string]bool)
	var lastErr error

	for attempt := 0; attempt <= p.cfg.MaxRetries; attempt++ {

		backend, err := p.GetNextBackend()
		if err != nil {
			lastErr = err
			continue
		}

		if tried[backend] {
			continue
		}
		tried[backend] = true

		ctx, cancel := context.WithTimeout(r.Context(), p.cfg.RequestTimeout)
		defer cancel()
		r = r.WithContext(ctx)

		targetURL, err := url.Parse(backend)
		if err != nil {
			lastErr = err
			continue
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.Transport = &http.Transport{
			ResponseHeaderTimeout: p.cfg.RequestTimeout,
		}

		proxy.ModifyResponse = func(resp *http.Response) error {
			if resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("backend %s failed with %d", backend, resp.StatusCode)
				return fmt.Errorf("retry")
			}
			return nil
		}

		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			lastErr = err
		}

		proxy.ServeHTTP(w, r)

		if lastErr == nil {
			// success
			log.Printf("[INFO] %s %s -> %s (%v)", r.Method, r.URL.Path, backend, time.Since(start))
			return
		}

		log.Printf("[RETRY] Attempt %d failed on %s: %v", attempt+1, backend, lastErr)
	}

	// retries exhausted
	http.Error(w, "All backends failed", http.StatusBadGateway)
	log.Printf("[FAIL] %s %s after %d attempts (%v)", r.Method, r.URL.Path, p.cfg.MaxRetries, lastErr)
}

func (p *ProxyServer) WatchConfig(path string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	err = watcher.Add(path)

	if err != nil {
		log.Fatalf("failed to watch file %s: %v", path, err)
	}

	log.Printf("[CONFIG] Watching %s for changes...", path)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				log.Printf("[CONFIG] Change detected: %s", event.Name)

				cfg, err := config.LoadConfig(path)
				if err != nil {
					log.Printf("[CONFIG] Failed to reload: %v", err)
					continue
				}

				// Apply the new config safely
				p.cfg = cfg
				log.Printf("[CONFIG] Reloaded successfully: %+v", cfg)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[CONFIG] Watcher error: %v", err)
		}
	}
}
