# BBProxy - 高速反向代理服务器

BBProxy 是一个用 Go 语言编写的高性能、功能丰富的反向代理服务器。它设计用于快速、可靠且易于配置，适用于小规模应用和大规模部署。

## 特性

- **TLS 支持**：使用 Let's Encrypt 自动管理 TLS 证书。
- **多后端支持**：支持多个后端服务器，可自定义路由。
- **断路器**：内置断路器，防止级联故障。
- **连接池**：高效的连接管理，提升性能。
- **指标收集**：全面的指标收集，用于监控和分析。
- **动态配置**：支持配置热重载，无需重启服务器。
- **负载均衡**：跨多个后端服务器的简单负载均衡。

## 安装

### 前提条件

- Go 1.16 或更高版本
- Git（用于克隆仓库）

### 从源码安装

1. 克隆仓库：
   ```bash
   git clone https://github.com/jacoblai/bbproxy.git
   cd bbproxy
   ```

2. 构建二进制文件：
   ```bash
   go build -o bbproxy
   ```

3. （可选）将二进制文件移动到 PATH 中的目录：
   ```bash
   sudo mv bbproxy /usr/local/bin/
   ```

## 配置

BBProxy 使用 JSON 配置文件。在二进制文件同目录下创建一个名为 `config.json` 的文件，或使用 `-config` 标志指定路径。

### 配置选项

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

- `domains`：域名到后端服务器地址的映射。
- `circuit_breaker_threshold`：断路器打开前的失败次数。
- `circuit_breaker_timeout`：断路器打开后尝试恢复的等待时间。
- `cert_cache_dir`：存储 Let's Encrypt 证书的目录。

## 使用方法

### 启动服务器

使用默认配置文件（当前目录下的 `config.json`）启动 BBProxy：

```bash
bbproxy
```

指定不同的配置文件：

```bash
bbproxy -config /path/to/your/config.json
```

### 重新加载配置

BBProxy 支持配置热重载。只需修改 `config.json` 文件，BBProxy 将自动检测变更并更新路由，无需重启。

### 监控

BBProxy 暴露了可以收集和监控的指标。默认情况下，这些指标每 60 秒记录一次。您可以将这些指标与您喜欢的监控解决方案集成。

## 高级用法

### 负载均衡

BBProxy 对给定域名的多个后端服务器执行简单的轮询负载均衡。要设置负载均衡，只需在配置中为一个域名指定多个后端地址：

```json
{
  "domains": {
    "example.com": ["localhost:8080", "localhost:8081", "localhost:8082"]
  }
}
```

### 断路器

断路器通过在后端重复失败时暂时禁用它来帮助防止级联故障。您可以使用 `circuit_breaker_threshold` 和 `circuit_breaker_timeout` 设置来配置其行为。

### TLS 证书

BBProxy 使用 Let's Encrypt 自动管理 TLS 证书。确保在配置中设置了 `cert_cache_dir`，并且 BBProxy 对该目录有写入权限。

## 故障排除

### 常见问题

1. **无法绑定到 443 端口**：确保您以足够的权限运行 BBProxy（例如，在类 Unix 系统上使用 `sudo`）。

2. **证书问题**：检查 `cert_cache_dir` 是否正确设置且可写。

3. **后端连接失败**：验证您的后端服务器是否正在运行，并且可以从代理服务器访问。

### 日志记录

BBProxy 记录重要事件和错误。默认情况下，日志写入 stdout。如果需要，您可以将其重定向到文件：

```bash
bbproxy > bbproxy.log 2>&1
```

## 性能调优

要从 BBProxy 获得最佳性能：

1. 增加进程可用的文件描述符数量。
2. 使用多个后端服务器并启用负载均衡。
3. 根据您的特定工作负载调整连接池大小。

## 贡献

欢迎对 BBProxy 做出贡献！以下是贡献方式：

1. Fork 仓库。
2. 为您的功能或 bug 修复创建一个新分支。
3. 编写代码和测试。
4. 提交一个 pull request，清楚描述您的更改。

请确保您的代码符合现有的风格并通过所有测试。

## 许可证

本项目采用 MIT 许可证 - 有关详细信息，请参阅 [LICENSE](LICENSE) 文件。

## 致谢

- 感谢所有为 BBProxy 开发做出贡献的贡献者。
- 本项目使用了几个优秀的 Go 库，包括 [fsnotify](https://github.com/fsnotify/fsnotify) 和 [go-metrics](https://github.com/rcrowley/go-metrics)。

## 支持和联系

如果您遇到任何问题或有疑问，请：

1. 查看 [Issues](https://github.com/jacoblai/bbproxy/issues) 页面，寻找现有问题或解决方案。
2. 如果找不到解决方案，请开启一个新的 issue，清楚描述问题和复现步骤。
3. 对于一般性问题或讨论，请在 [Discussions](https://github.com/jacoblai/bbproxy/discussions) 中开启一个新话题。

对于安全相关的问题，请发送邮件至 security@yourdomain.com，而不是开启公开的 issue。
```