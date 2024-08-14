```markdown
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

To install BBProxy, make sure you have Go 1.16 or later installed, then run:

```bash
go get github.com/jacoblai/bbproxy
```

## Quick Start

1. Create a configuration file named `config.json`:

```json
{
  "domains": {
    "example.com": ["localhost:8080", "localhost:8081"],
    "another-example.com": ["localhost:9090"]
  },
  "circuit_breaker_threshold": 5,
  "circuit_breaker_timeout": "30s"
}
```

2. Run the proxy server:

```bash
bbproxy
```

The server will start and listen on port 443 for HTTPS connections.

## Configuration

BBProxy uses a JSON configuration file. Here's an explanation of the configuration options:

- `domains`: A map of domain names to backend server addresses.
- `circuit_breaker_threshold`: Number of failures before the circuit breaker opens.
- `circuit_breaker_timeout`: Duration the circuit breaker stays open before trying again.

## Building from Source

To build BBProxy from source:

```bash
git clone https://github.com/jacoblai/bbproxy.git
cd bbproxy
go build
```

## Running Tests

To run the test suite:

```bash
go test ./...
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Thanks to all contributors who have helped with the development of BBProxy.
- This project makes use of several excellent Go libraries, including [fsnotify](https://github.com/fsnotify/fsnotify) and [go-metrics](https://github.com/rcrowley/go-metrics).

## Contact

If you have any questions or feedback, please open an issue on the GitHub repository.
```