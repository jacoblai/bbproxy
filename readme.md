# BBProxy - Blazing Fast Reverse Proxy Server

BBProxy is a high-performance, feature-rich reverse proxy server written in Go. It's designed to be fast, reliable, and easy to configure, making it perfect for both small-scale applications and large-scale deployments.

## Features

- **TLS Support**: Automatic TLS certificate management using Let's Encrypt.
- **Multiple Backends**: Support for multiple backend servers with customizable routing.
- **Circuit Breaker**: Built-in circuit breaker to prevent cascading failures.
- **Connection Pooling**: Efficient connection management for improved performance.
- **Metrics**: Comprehensive metrics collection for monitoring and analysis.
- **Dynamic Configuration**: Hot-reload configuration without restarting the server.
- **Load Balancing**: Simple load balancing across multiple backend servers.

## Installation

### Prerequisites

- Go 1.16 or later
- Git (for cloning the repository)

### Installing from Source

1. Clone the repository:
   ```bash
   git clone https://github.com/jacoblai/bbproxy.git
   cd bbproxy
   ```

2. Build the binary:
   ```bash
   go build -o bbproxy
   ```

3. (Optional) Move the binary to a directory in your PATH:
   ```bash
   sudo mv bbproxy /usr/local/bin/
   ```

## Configuration

BBProxy uses a JSON configuration file. Create a file named `config.json` in the same directory as the binary, or specify a path using the `-config` flag.

### Configuration Options

```json
{
  "domains": {
    "example.com": ["localhost:8080", "localhost:8081"],
    "another-example.com": ["localhost:9090"]
  },
  "circuit_breaker_threshold": 5,
  "circuit_breaker_timeout": "30s",
  "cert_cache_dir": "/path/to/cert/cache"
}
```

- `domains`: A map of domain names to backend server addresses.
- `circuit_breaker_threshold`: Number of failures before the circuit breaker opens.
- `circuit_breaker_timeout`: Duration the circuit breaker stays open before trying again.
- `cert_cache_dir`: Directory to store Let's Encrypt certificates.

## Usage

### Starting the Server

To start BBProxy with the default configuration file (`config.json` in the current directory):

```bash
bbproxy
```

To specify a different configuration file:

```bash
bbproxy -config /path/to/your/config.json
```

### Reloading Configuration

BBProxy supports hot reloading of its configuration. Simply modify the `config.json` file, and BBProxy will automatically detect the changes and update its routing without restarting.

### Monitoring

BBProxy exposes metrics that can be collected and monitored. By default, these metrics are logged every 60 seconds. You can integrate these metrics with your preferred monitoring solution.

## Advanced Usage

### Load Balancing

BBProxy performs simple round-robin load balancing across multiple backend servers for a given domain. To set up load balancing, simply specify multiple backend addresses for a domain in the configuration:

```json
{
  "domains": {
    "example.com": ["localhost:8080", "localhost:8081", "localhost:8082"]
  }
}
```

### Circuit Breaker

The circuit breaker helps prevent cascading failures by temporarily disabling a backend if it fails repeatedly. You can configure its behavior using the `circuit_breaker_threshold` and `circuit_breaker_timeout` settings.

### TLS Certificates

BBProxy automatically manages TLS certificates using Let's Encrypt. Ensure that the `cert_cache_dir` is set in your configuration and that BBProxy has write permissions to this directory.

## Troubleshooting

### Common Issues

1. **Unable to bind to port 443**: Ensure you're running BBProxy with sufficient privileges (e.g., using `sudo` on Unix-like systems).

2. **Certificate issues**: Check that the `cert_cache_dir` is correctly set and writable.

3. **Backend connection failures**: Verify that your backend servers are running and accessible from the proxy server.

### Logging

BBProxy logs important events and errors. By default, logs are written to stdout. You can redirect these to a file if needed:

```bash
bbproxy > bbproxy.log 2>&1
```

## Performance Tuning

To get the best performance out of BBProxy:

1. Increase the number of file descriptors available to the process.
2. Use multiple backend servers and enable load balancing.
3. Adjust the connection pool size based on your specific workload.

## Contributing

Contributions to BBProxy are welcome! Here's how you can contribute:

1. Fork the repository.
2. Create a new branch for your feature or bug fix.
3. Write your code and tests.
4. Submit a pull request with a clear description of your changes.

Please ensure your code adheres to the existing style and passes all tests.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Thanks to all contributors who have helped with the development of BBProxy.
- This project makes use of several excellent Go libraries, including [fsnotify](https://github.com/fsnotify/fsnotify) and [go-metrics](https://github.com/rcrowley/go-metrics).

## Support and Contact

If you encounter any issues or have questions, please:

1. Check the [Issues](https://github.com/jacoblai/bbproxy/issues) page for existing problems or solutions.
2. If you can't find a solution, open a new issue with a clear description of the problem and steps to reproduce it.
3. For general questions or discussions, open a [Discussion](https://github.com/jacoblai/bbproxy/discussions) topic.

For security-related issues, please email security@yourdomain.com instead of opening a public issue.
```