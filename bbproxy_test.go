package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// 创建一个临时配置文件
	tempFile, err := ioutil.TempFile("", "config*.json")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	// 写入测试配置
	testConfig := Config{
		Domains: map[string][]string{
			"example.com": {"localhost:8080", "localhost:8081"},
		},
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30 * time.Second,
	}
	configData, err := json.Marshal(testConfig)
	require.NoError(t, err)
	_, err = tempFile.Write(configData)
	require.NoError(t, err)
	tempFile.Close()

	// 测试加载配置
	loadedConfig, err := loadConfig(tempFile.Name())
	assert.NoError(t, err)
	assert.Equal(t, testConfig.Domains, loadedConfig.Domains)
	assert.Equal(t, testConfig.CircuitBreakerThreshold, loadedConfig.CircuitBreakerThreshold)
	assert.Equal(t, testConfig.CircuitBreakerTimeout, loadedConfig.CircuitBreakerTimeout)
}

func TestNewBackendManager(t *testing.T) {
	config := &Config{
		Domains: map[string][]string{
			"example.com": {"localhost:8080", "localhost:8081"},
		},
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30 * time.Second,
	}

	bm, err := NewBackendManager(config)
	assert.NoError(t, err)
	assert.NotNil(t, bm)
	assert.Len(t, bm.domains["example.com"], 2)
	assert.NotNil(t, bm.sessionCache)
}

func TestBackendManagerChooseBackend(t *testing.T) {
	config := &Config{
		Domains: map[string][]string{
			"example.com": {"localhost:8080", "localhost:8081"},
		},
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30 * time.Second,
	}

	bm, err := NewBackendManager(config)
	require.NoError(t, err)

	ctx := context.Background()
	backend, err := bm.ChooseBackend(ctx, "example.com")
	assert.NoError(t, err)
	assert.NotNil(t, backend)
	assert.Contains(t, []string{"localhost:8080", "localhost:8081"}, backend.Address)

	_, err = bm.ChooseBackend(ctx, "nonexistent.com")
	assert.Error(t, err)
}

func TestConnPool(t *testing.T) {
	// 创建一个模拟的 TCP 服务器
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer listener.Close()

	serverAddr := listener.Addr().String()

	// 在后台运行服务器
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close() // 立即关闭连接，因为我们只是在测试连接池
		}
	}()

	pool := NewConnPool(2, serverAddr)
	assert.NotNil(t, pool)

	ctx := context.Background()

	// 测试获取连接
	conn1, err := pool.Get(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, conn1)

	conn2, err := pool.Get(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, conn2)

	// 池已满，应该创建新连接
	conn3, err := pool.Get(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, conn3)

	// 测试放回连接
	if conn1 != nil {
		pool.Put(conn1)
	}
	if conn2 != nil {
		pool.Put(conn2)
	}
	if conn3 != nil {
		pool.Put(conn3)
	}

	// 测试关闭所有连接
	pool.CloseAll(ctx)
}

func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Second)
	assert.NotNil(t, cb)

	// 初始状态应该是关闭的
	assert.True(t, cb.Allow())

	// 记录3次失败，应该打开
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	assert.False(t, cb.Allow())

	// 等待超时后，应该进入半开状态
	time.Sleep(1100 * time.Millisecond)
	assert.True(t, cb.Allow())

	// 记录成功，应该关闭
	cb.RecordSuccess()
	assert.True(t, cb.Allow())
}

func TestProxyServer(t *testing.T) {
	// 生成测试证书
	certPEM, keyPEM, err := generateTestCert()
	require.NoError(t, err, "Failed to generate test certificate")

	// 创建一个模拟的后端服务器
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))
	defer backend.Close()

	// 创建模拟配置
	config := &Config{
		Domains: map[string][]string{
			"example.com": {backend.URL[7:]}, // 移除 "http://" 前缀
		},
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30 * time.Second,
	}

	// 创建代理服务器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := NewProxyServer(ctx, config)
	require.NoError(t, err)

	// 创建一个测试用的 TLS 配置
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err, "Failed to create X509 key pair")

	server.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// 创建一个监听器
	listener, err := tls.Listen("tcp", "localhost:0", server.tlsConfig)
	require.NoError(t, err)
	defer listener.Close()

	// 在一个新的 goroutine 中运行服务器
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.serveHTTPS(ctx)
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建一个 HTTPS 客户端
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	// 发送请求到代理服务器
	url := "https://" + listener.Addr().String()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	require.NoError(t, err)
	req.Host = "example.com"

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", string(body))

	// 取消上下文以停止服务器
	cancel()

	// 等待服务器退出或超时
	select {
	case err := <-serverErrCh:
		if err != nil && err != context.Canceled {
			t.Errorf("serveHTTPS returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Server did not shut down within the expected time")
	}
}
func TestMetricsReporting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	config := &Config{
		Domains: map[string][]string{
			"example.com": {"localhost:8080"},
		},
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30 * time.Second,
	}

	server, err := NewProxyServer(ctx, config)
	require.NoError(t, err)

	backend := server.backendManager.domains["example.com"][0]
	backend.Metrics.RequestCount.Inc(10)
	backend.Metrics.ErrorCount.Inc(2)
	backend.Metrics.ResponseTime.Update(100 * time.Millisecond)
	backend.Metrics.ResponseTime.Update(150 * time.Millisecond)

	err = server.reportMetrics(ctx)
	assert.Error(t, err) // 应该返回 context.DeadlineExceeded 错误

	// 验证度量数据
	assert.Equal(t, int64(10), backend.Metrics.RequestCount.Count())
	assert.Equal(t, int64(2), backend.Metrics.ErrorCount.Count())
	assert.Equal(t, time.Duration(100*time.Millisecond).Nanoseconds(), backend.Metrics.ResponseTime.Min())
	assert.Equal(t, time.Duration(150*time.Millisecond).Nanoseconds(), backend.Metrics.ResponseTime.Max())
}

func generateTestCert() (certPEM, keyPEM []byte, err error) {
	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// 准备证书模板
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // 1年有效期

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Test Co"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	// 创建自签名证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	// 将证书编码为PEM格式
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	// 将私钥编码为PEM格式
	privateKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}
	keyPEM = pem.EncodeToMemory(privateKeyPEM)

	return certPEM, keyPEM, nil
}
