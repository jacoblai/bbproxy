package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rcrowley/go-metrics"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/sync/errgroup"
)

type Config struct {
	Domains                 map[string][]string `json:"domains"`
	CircuitBreakerThreshold int                 `json:"circuit_breaker_threshold"`
	CircuitBreakerTimeout   time.Duration       `json:"circuit_breaker_timeout"`
	CertCachePath           string              `json:"cert_cache_path"` // 新增字段
}

type ProxyServer struct {
	config          *Config
	backendManager  *BackendManager
	certManager     *autocert.Manager
	tlsConfig       *tls.Config
	metricsRegistry metrics.Registry
	watcher         *fsnotify.Watcher
	httpServer      *http.Server
}

type BackendManager struct {
	domains      map[string][]*Backend
	mu           sync.RWMutex
	sessionCache tls.ClientSessionCache
}

type Backend struct {
	Address  string
	Healthy  bool
	ConnPool *ConnPool
	Circuit  *CircuitBreaker
	Metrics  *BackendMetrics
}

type ConnPool struct {
	mu       sync.Mutex
	conns    chan net.Conn
	maxConns int
	address  string
}

type CircuitBreaker struct {
	mu           sync.Mutex
	failureCount int
	lastFailure  time.Time
	state        int
	threshold    int
	timeout      time.Duration
}

type BackendMetrics struct {
	RequestCount metrics.Counter
	ErrorCount   metrics.Counter
	ResponseTime metrics.Timer
}

func (s *ProxyServer) initHTTPServer() {
	mux := http.NewServeMux()
	mux.Handle("/.well-known/acme-challenge/", s.certManager.HTTPHandler(nil))

	s.httpServer = &http.Server{
		Addr:    ":80",
		Handler: mux,
	}
}

func NewProxyServer(ctx context.Context, config *Config) (*ProxyServer, error) {
	backendManager, err := NewBackendManager(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create backend manager: %v", err)
	}

	server := &ProxyServer{
		config:          config,
		backendManager:  backendManager,
		metricsRegistry: metrics.NewRegistry(),
	}

	if err := server.initCertManager(); err != nil {
		return nil, fmt.Errorf("failed to initialize cert manager: %v", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %v", err)
	}
	server.watcher = watcher

	return server, nil
}

func (s *ProxyServer) initCertManager() error {
	var domains []string
	for domain := range s.config.Domains {
		domains = append(domains, domain)
	}

	s.certManager = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domains...),
		Cache:      autocert.DirCache(s.config.CertCachePath), // 使用配置中的路径
	}

	s.tlsConfig = &tls.Config{
		GetCertificate:     s.certManager.GetCertificate,
		ClientSessionCache: s.backendManager.sessionCache,
	}

	return nil
}

func (s *ProxyServer) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	// 启动 HTTPS 服务
	g.Go(func() error {
		return s.serveHTTPS(ctx)
	})

	// 启动 HTTP 服务（用于 ACME 挑战）
	g.Go(func() error {
		log.Println("Starting HTTP server on :80 for ACME challenges")
		err := s.httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("HTTP server error: %v", err)
		}
		return nil
	})

	g.Go(func() error {
		return s.watchConfig(ctx)
	})

	g.Go(func() error {
		return s.reportMetrics(ctx)
	})

	return g.Wait()
}

func (s *ProxyServer) serveHTTPS(ctx context.Context) error {
	listener, err := tls.Listen("tcp", ":443", s.tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}
	defer listener.Close()

	log.Println("Starting TLS proxy on :443")

	errCh := make(chan error, 1)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				default:
					errCh <- fmt.Errorf("failed to accept connection: %v", err)
					return
				}
			}
			go s.handleConnection(ctx, conn)
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *ProxyServer) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	tlsConn, ok := clientConn.(*tls.Conn)
	if !ok {
		log.Println("Failed to cast to TLS connection")
		return
	}

	if err := tlsConn.HandshakeContext(ctx); err != nil {
		log.Printf("TLS handshake failed: %v", err)
		return
	}

	serverName := tlsConn.ConnectionState().ServerName
	backend, err := s.backendManager.ChooseBackend(ctx, serverName)
	if err != nil {
		log.Printf("Failed to choose backend for %s: %v", serverName, err)
		return
	}

	backendConn, err := backend.ConnPool.GetWithRetry(ctx)
	if err != nil {
		log.Printf("Failed to connect to backend %s: %v", backend.Address, err)
		backend.Circuit.RecordFailure()
		backend.Metrics.ErrorCount.Inc(1)
		return
	}
	defer backend.ConnPool.Put(backendConn)

	backend.Metrics.RequestCount.Inc(1)
	startTime := time.Now()

	err = s.proxyData(ctx, tlsConn, backendConn)
	if err != nil {
		log.Printf("Error proxying data: %v", err)
		backend.Circuit.RecordFailure()
		backend.Metrics.ErrorCount.Inc(1)
	} else {
		backend.Circuit.RecordSuccess()
	}

	backend.Metrics.ResponseTime.UpdateSince(startTime)
}

func (s *ProxyServer) proxyData(ctx context.Context, client, backend net.Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var g errgroup.Group

	g.Go(func() error {
		_, err := io.Copy(backend, client)
		if err != nil {
			cancel()
		}
		backend.(interface{ CloseWrite() error }).CloseWrite()
		return err
	})

	g.Go(func() error {
		_, err := io.Copy(client, backend)
		if err != nil {
			cancel()
		}
		client.(interface{ CloseWrite() error }).CloseWrite()
		return err
	})

	return g.Wait()
}

func (s *ProxyServer) watchConfig(ctx context.Context) error {
	err := s.watcher.Add("config.json")
	if err != nil {
		return fmt.Errorf("failed to watch config file: %v", err)
	}

	for {
		select {
		case event, ok := <-s.watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				log.Println("Config file modified. Reloading...")
				if err := s.reloadConfig(ctx); err != nil {
					log.Printf("Failed to reload config: %v", err)
				}
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("Error watching config file: %v", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *ProxyServer) reloadConfig(ctx context.Context) error {
	newConfig, err := loadConfig("config.json")
	if err != nil {
		return fmt.Errorf("failed to load new config: %v", err)
	}

	newBackendManager, err := NewBackendManager(newConfig)
	if err != nil {
		return fmt.Errorf("failed to create new backend manager: %v", err)
	}

	// Gracefully switch to new backend manager
	oldBackendManager := s.backendManager
	s.backendManager = newBackendManager
	s.config = newConfig

	// Update TLS config
	if err := s.initCertManager(); err != nil {
		return fmt.Errorf("failed to reinitialize cert manager: %v", err)
	}

	// Close old backend connections
	oldBackendManager.CloseAll(ctx)

	log.Println("Config reloaded successfully")
	return nil
}

func (s *ProxyServer) reportMetrics(ctx context.Context) error {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.metricsRegistry.Each(func(name string, i interface{}) {
				switch metric := i.(type) {
				case metrics.Counter:
					log.Printf("Counter %s: count: %d", name, metric.Count())
				case metrics.Gauge:
					log.Printf("Gauge %s: value: %d", name, metric.Value())
				case metrics.Timer:
					ps := metric.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
					log.Printf("Timer %s: min: %d ns, max: %d ns, mean: %.2f ns, median: %.2f ns, 95%%: %.2f ns, 99%%: %.2f ns",
						name,
						metric.Min(),
						metric.Max(),
						float64(metric.Mean()),
						ps[0],
						ps[2],
						ps[3])
				}
			})
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func NewBackendManager(config *Config) (*BackendManager, error) {
	bm := &BackendManager{
		domains:      make(map[string][]*Backend),
		sessionCache: tls.NewLRUClientSessionCache(10000),
	}

	for domain, addresses := range config.Domains {
		var backends []*Backend
		for _, addr := range addresses {
			backend, err := NewBackend(addr, config.CircuitBreakerThreshold, config.CircuitBreakerTimeout)
			if err != nil {
				return nil, fmt.Errorf("failed to create backend for %s: %v", addr, err)
			}
			backends = append(backends, backend)
		}
		bm.domains[domain] = backends
	}

	return bm, nil
}

func (bm *BackendManager) ChooseBackend(ctx context.Context, domain string) (*Backend, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	backends, ok := bm.domains[domain]
	if !ok || len(backends) == 0 {
		return nil, fmt.Errorf("no backend available for domain %s", domain)
	}

	// Implement load balancing logic here
	// For simplicity, we're just returning the first healthy backend
	for _, backend := range backends {
		if backend.Healthy && backend.Circuit.Allow() {
			return backend, nil
		}
	}

	return nil, fmt.Errorf("no healthy backend available for domain %s", domain)
}

func (bm *BackendManager) CloseAll(ctx context.Context) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, backends := range bm.domains {
		for _, backend := range backends {
			backend.ConnPool.CloseAll(ctx)
		}
	}
}

func NewBackend(address string, circuitThreshold int, circuitTimeout time.Duration) (*Backend, error) {
	return &Backend{
		Address:  address,
		Healthy:  true,
		ConnPool: NewConnPool(100, address),
		Circuit:  NewCircuitBreaker(circuitThreshold, circuitTimeout),
		Metrics:  NewBackendMetrics(address),
	}, nil
}

func NewConnPool(maxConns int, address string) *ConnPool {
	return &ConnPool{
		conns:    make(chan net.Conn, maxConns),
		maxConns: maxConns,
		address:  address,
	}
}

func (p *ConnPool) Get(ctx context.Context) (net.Conn, error) {
	select {
	case conn := <-p.conns:
		return conn, nil
	default:
		return net.DialTimeout("tcp", p.address, 5*time.Second)
	}
}

func (p *ConnPool) GetWithRetry(ctx context.Context) (net.Conn, error) {
	for i := 0; i < 3; i++ {
		conn, err := p.Get(ctx)
		if err == nil {
			return conn, nil
		}
		log.Printf("Failed to get connection from pool, attempt %d: %v", i+1, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(i+1) * time.Second):
			// Exponential backoff
		}
	}
	return nil, fmt.Errorf("failed to get connection after 3 attempts")
}

func (p *ConnPool) Put(conn net.Conn) {
	if conn == nil {
		return // 忽略 nil 连接
	}
	select {
	case p.conns <- conn:
	default:
		conn.Close()
	}
}

func (p *ConnPool) CloseAll(ctx context.Context) {
	close(p.conns)
	for conn := range p.conns {
		if conn != nil {
			conn.Close()
		}
	}
}

func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		timeout:   timeout,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case 0: // closed
		return true
	case 1: // open
		if time.Since(cb.lastFailure) > cb.timeout {
			cb.state = 2 // half-open
			return true
		}
		return false
	case 2: // half-open
		return true
	}
	return false
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.state = 0 // closed
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	if cb.failureCount >= cb.threshold {
		cb.state = 1 // open
		cb.lastFailure = time.Now()
	}
}

func NewBackendMetrics(address string) *BackendMetrics {
	return &BackendMetrics{
		RequestCount: metrics.NewCounter(),
		ErrorCount:   metrics.NewCounter(),
		ResponseTime: metrics.NewTimer(),
	}
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// 可以在这里添加配置验证
	if config.CertCachePath == "" {
		return nil, fmt.Errorf("cert_cache_path is required in the configuration")
	}

	return &config, nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	server, err := NewProxyServer(ctx, config)
	if err != nil {
		log.Fatalf("Failed to create proxy server: %v", err)
	}

	server.initHTTPServer()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
		cancel()
	}()

	if err := server.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("Server error: %v", err)
	}
}

func (s *ProxyServer) Shutdown(ctx context.Context) error {
	// 关闭 HTTPS 监听器（假设你有一个方法来做这个）
	// 如果没有，你可能需要重构 serveHTTPS 方法以便能够优雅关闭

	// 关闭 HTTP 服务器
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("HTTP server shutdown error: %v", err)
	}

	// 关闭其他资源
	s.watcher.Close()
	s.backendManager.CloseAll(ctx)

	return nil
}
