# yourtestsrv

一个供嵌入式设备访问的网络服务器，用于测试 TCP/UDP/HTTP/MQTT 等协议。

## 特性

- **自定义协议实现**: 不使用传统 HTTP/MQTT 库，可完全控制协议行为
- **多种协议支持**: TCP、UDP、HTTP、MQTT
- **加密/非加密**: 所有协议同时开启，TLS 端口 = 非加密端口 + 10000
- **无外部依赖**: 命令行与配置解析均使用标准库
- **特殊场景**: 包含各种边界情况和错误场景，用于测试嵌入式设备

## 协议支持

### TCP
- 简单回显服务器
- 延迟响应 (可配置延迟)
- 连接断开模拟
- 错误响应
- 半关闭连接

### UDP
- 简单回显
- 包丢失模拟
- 乱序发送
- 延迟发送

### HTTP
- 自定义 HTTP 解析器
- 各种 HTTP 方法 (GET, POST, PUT, DELETE, etc)
- 分块传输 (Chunked Transfer)
- 不完整响应
- 慢响应
- 错误状态码
- 特殊 Header 处理
- 断点续传

### MQTT
- 自定义 MQTT 解析器 (MQTT 3.1.1 / 5.0)
- 各种 QoS 级别
- 遗嘱消息
- 保留消息
- 客户端 ID 验证
- 异常包处理

## 运行

由于本项目使用纯 Python 3 编写，无需编译即可直接运行：

```bash
python yourtestsrv.py --help
```

> **注意**：建议使用 Python 3.11 或以上版本。由于全部使用 Python 标准库，因此无需安装任何第三方运行时依赖（测试除外）。

## 使用方法

```bash
python yourtestsrv.py --help
```

### 部署说明

- systemd: `docs/systemd.md`
- Docker: `docs/docker.md`

### 启动所有服务 (非加密)

```bash
python yourtestsrv.py serve-all --config config.json

# 仅监听本机
python yourtestsrv.py serve-all --bind 127.0.0.1 --config config.json
```

### 启动所有服务 (加密)

```bash
python yourtestsrv.py serve-all-tls --config config.json
```

### 启动单个服务

```bash
# TCP
python yourtestsrv.py tcp --port 9000 --config config.json

# TCP TLS
python yourtestsrv.py tcp --port 9443 --tls --config config.json

# UDP
python yourtestsrv.py udp --port 9001 --config config.json

# HTTP
python yourtestsrv.py http --port 8080 --config config.json

# HTTP TLS
python yourtestsrv.py http --port 8443 --tls --config config.json

# MQTT
python yourtestsrv.py mqtt --port 1883 --config config.json

# MQTT TLS
python yourtestsrv.py mqtt --port 8883 --tls --config config.json
```

### 特殊场景选项

```bash
# TCP 延迟响应 (5秒)
python yourtestsrv.py tcp --port 9000 --delay 5s --config config.json

# TCP 主动断开连接
python yourtestsrv.py tcp --port 9000 --close-after 3s --config config.json

# HTTP 慢响应
python yourtestsrv.py http --port 8080 --slow-response --slow-duration 30s --config config.json

# HTTP 错误状态码
python yourtestsrv.py http --port 8080 --error-code 500 --config config.json

# UDP 包丢失模拟 (50%)
python yourtestsrv.py udp --port 9001 --drop-rate 0.5 --config config.json

# MQTT 保留消息
python yourtestsrv.py mqtt --port 1883 --retain --config config.json
```

## 配置

也可以通过配置文件 (config.json) 进行配置:

```json
{
  "server": {
    "bind": "0.0.0.0",
    "tcp": {
      "port": 9000,
      "delay": "0s",
      "close_after": "0s"
    },
    "udp": {
      "port": 9001,
      "drop_rate": 0,
      "delay": "0s"
    },
    "http": {
      "port": 8080,
      "slow_response": false,
      "slow_duration": "0s",
      "error_code": 200,
      "chunked": false
    },
    "mqtt": {
      "port": 1883,
      "retain": false
    }
  },
  "logging": {
    "level": "info"
  }
}
```

## 证书生成

生成自签名证书用于测试:

```bash
# 生成 RSA 证书
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes

# 生成 ECDSA 证书
openssl ecparam -genkey -name prime256v1 -out key.pem
openssl req -new -key key.pem -x509 -days 365 -out cert.pem
```

## 测试示例

### TCP 测试

```bash
# 简单连接测试
nc localhost 9000
hello

# TLS 连接测试
openssl s_client -connect localhost:9443
```

### HTTP 测试

```bash
# GET 请求
curl http://localhost:8080/

# 健康检查
curl http://localhost:8080/healthz

# 慢响应测试
curl -w "%{time_total}\n" http://localhost:8080/slow

# 错误码测试
curl -i http://localhost:8080/error/500
```

### MQTT 测试

```bash
# 使用 mosquitto_sub/pub
mosquitto_sub -t "test/#" -v
mosquitto_pub -t "test/hello" -m "world"

# TLS 连接
mosquitto_sub -t "test/#" -v --cafile cert.pem
```

## 目录结构

```
yourtestsrv/
├── yourtestsrv.py        # CLI 入口与服务器启动
├── yourtestsrv/          # 核心模块
│   ├── __init__.py
│   ├── config.py
│   ├── tcp_server.py
│   ├── udp_server.py
│   ├── http_server.py
│   └── mqtt_server.py
├── tests/                # pytest 测试集
├── config.json           # 默认配置文件示例
├── README.md
└── CHANGELOG.md
```
