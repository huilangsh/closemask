# CloseMask 部署指南

本文档提供 CloseMask 的详细部署说明，包括不同环境下的最佳实践。

## 目录

- [系统要求](#系统要求)
- [本地开发环境](#本地开发环境)
- [生产环境](#生产环境)
- [容器化部署](#容器化部署)
- [Kubernetes 部署](#kubernetes-部署)
- [配置管理](#配置管理)
- [监控和日志](#监控和日志)
- [故障排查](#故障排查)

## 系统要求

### 最低配置

- **CPU**: 2 核
- **内存**: 4GB
- **磁盘**: 20GB
- **操作系统**: Linux/macOS/Windows

### 推荐配置（生产环境）

- **CPU**: 4 核
- **内存**: 8GB
- **磁盘**: 50GB SSD
- **网络**: 1Gbps

### 依赖服务

- **OneAIFW**: PII 检测引擎（必需）
- **LLM 提供商**: OpenAI 兼容的 API（必需）
- **Redis**: 可选，用于分布式会话存储

## 本地开发环境

### 方式 1: 直接运行

```bash
# 克隆仓库
git clone https://github.com/huilangsh/closemask.git
cd closemask

# 编译
go build -o closemask ./cmd/proxy

# 运行
./closemask
```

### 方式 2: 使用 Docker Compose

创建 `docker-compose.yml`:

```yaml
version: '3.8'

services:
  closemask:
    build: .
    ports:
      - "8846:8846"
    environment:
      - CLOSEMASK_ONEAIFW_URL=http://oneaifw:8844
      - CLOSEMASK_LLM_API_KEY=your-api-key
    depends_on:
      - oneaifw
      - redis

  oneaifw:
    image: funstory/aifw:latest
    ports:
      - "8844:8844"
    volumes:
      - ./aifw-data:/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - ./redis-data:/data
```

启动服务：

```bash
docker-compose up -d
```

## 生产环境

### 系统级安装

#### 1. 下载二进制文件

```bash
# 从 GitHub Releases 下载最新版本
wget https://github.com/huilangsh/closemask/releases/latest/download/closemask-linux-amd64

# 添加执行权限
chmod +x closemask-linux-amd64

# 移动到系统路径
sudo mv closemask-linux-amd64 /usr/local/bin/closemask
```

#### 2. 创建配置文件

```bash
sudo mkdir -p /etc/closemask
sudo nano /etc/closemask/config.json
```

配置示例：

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8846
  },
  "oneaifw": {
    "url": "http://localhost:8844",
    "timeout": "10s"
  },
  "llm": {
    "baseUrl": "https://api.openai.com/v1",
    "apiKey": "your-production-api-key",
    "defaultModel": "gpt-3.5-turbo"
  },
  "session": {
    "ttl": "1h",
    "maxSize": 10000,
    "storageType": "redis",
    "redisAddr": "localhost:6379"
  }
}
```

#### 3. 创建 systemd 服务

创建 `/etc/systemd/system/closemask.service`:

```ini
[Unit]
Description=CloseMask PII Proxy
After=network.target oneaifw.service redis.service

[Service]
Type=simple
User=closemask
Group=closemask
WorkingDirectory=/opt/closemask
ExecStart=/usr/local/bin/closemask -config /etc/closemask/config.json
Restart=on-failure
RestartSec=5s

# 安全设置
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true

# 资源限制
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

#### 4. 创建用户和目录

```bash
# 创建专用用户
sudo useradd -r -s /bin/false closemask

# 创建目录
sudo mkdir -p /opt/closemask
sudo chown closemask:closemask /opt/closemask

# 创建日志目录
sudo mkdir -p /var/log/closemask
sudo chown closemask:closemask /var/log/closemask
```

#### 5. 启动服务

```bash
# 重新加载 systemd 配置
sudo systemctl daemon-reload

# 启用开机自启
sudo systemctl enable closemask

# 启动服务
sudo systemctl start closemask

# 查看状态
sudo systemctl status closemask

# 查看日志
sudo journalctl -u closemask -f
```

## 容器化部署

### 构建 Docker 镜像

#### Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 编译
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o closemask ./cmd/proxy

# 运行时镜像
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# 从构建阶段复制二进制文件
COPY --from=builder /app/closemask .

# 复制配置文件
COPY config.json .

EXPOSE 8846

CMD ["./closemask"]
```

#### 构建镜像

```bash
# 构建镜像
docker build -t closemask:latest .

# 使用构建参数
docker build \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -t closemask:1.0.0 .
```

### 运行容器

#### 基本运行

```bash
docker run -d \
  --name closemask \
  -p 8846:8846 \
  -e CLOSEMASK_ONEAIFW_URL=http://host.docker.internal:8844 \
  -e CLOSEMASK_LLM_API_KEY=your-api-key \
  closemask:latest
```

#### 带卷挂载

```bash
docker run -d \
  --name closemask \
  -p 8846:8846 \
  -v /opt/closemask/config.json:/app/config.json:ro \
  -v /var/log/closemask:/var/log/closemask \
  -e CLOSEMASK_ONEAIFW_URL=http://oneaifw:8844 \
  closemask:latest
```

#### 使用 Docker Compose

```yaml
version: '3.8'

services:
  closemask:
    image: closemask:latest
    container_name: closemask
    restart: unless-stopped
    ports:
      - "8846:8846"
    environment:
      - CLOSEMASK_ONEAIFW_URL=http://oneaifw:8844
      - CLOSEMASK_LLM_API_KEY=${LLM_API_KEY}
      - CLOSEMASK_REDIS_ADDR=redis:6379
    volumes:
      - ./config.json:/app/config.json:ro
      - closemask-logs:/var/log/closemask
    depends_on:
      - oneaifw
      - redis
    networks:
      - closemask-network
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8846/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  oneaifw:
    image: funstory/aifw:latest
    container_name: oneaifw
    restart: unless-stopped
    ports:
      - "8844:8844"
    volumes:
      - oneaifw-data:/data
    networks:
      - closemask-network

  redis:
    image: redis:7-alpine
    container_name: closemask-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    command: redis-server --appendonly yes
    networks:
      - closemask-network

volumes:
  closemask-logs:
  oneaifw-data:
  redis-data:

networks:
  closemask-network:
    driver: bridge
```

## Kubernetes 部署

### 部署配置

#### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: closemask
  labels:
    app: closemask
spec:
  replicas: 3
  selector:
    matchLabels:
      app: closemask
  template:
    metadata:
      labels:
        app: closemask
    spec:
      containers:
      - name: closemask
        image: closemask:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 8846
          name: http
          protocol: TCP
        env:
        - name: CLOSEMASK_ONEAIFW_URL
          value: "http://oneaifw-service:8844"
        - name: CLOSEMASK_LLM_API_KEY
          valueFrom:
            secretKeyRef:
              name: closemask-secrets
              key: llm-api-key
        - name: CLOSEMASK_REDIS_ADDR
          value: "redis-service:6379"
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8846
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health
            port: 8846
          initialDelaySeconds: 10
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        volumeMounts:
        - name: config
          mountPath: /app/config.json
          subPath: config.json
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: closemask-config
```

#### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: closemask-service
  labels:
    app: closemask
spec:
  type: ClusterIP
  ports:
  - port: 8846
    targetPort: 8846
    protocol: TCP
    name: http
  selector:
    app: closemask
```

#### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: closemask-config
data:
  config.json: |
    {
      "server": {
        "host": "0.0.0.0",
        "port": 8846
      },
      "oneaifw": {
        "url": "http://oneaifw-service:8844",
        "timeout": "10s"
      },
      "llm": {
        "baseUrl": "https://api.openai.com/v1",
        "defaultModel": "gpt-3.5-turbo"
      },
      "session": {
        "ttl": "1h",
        "maxSize": 10000,
        "storageType": "redis",
        "redisAddr": "redis-service:6379"
      }
    }
```

#### Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: closemask-secrets
type: Opaque
stringData:
  llm-api-key: your-actual-api-key
```

#### HorizontalPodAutoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: closemask-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: closemask
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

#### PodDisruptionBudget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: closemask-pdb
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: closemask
```

### 部署到集群

```bash
# 应用配置
kubectl apply -f k8s/

# 查看部署状态
kubectl get pods -l app=closemask

# 查看服务
kubectl get svc closemask-service

# 查看日志
kubectl logs -f deployment/closemask

# 扩展副本
kubectl scale deployment closemask --replicas=5
```

## 配置管理

### 环境变量

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `CLOSEMASK_CONFIG` | 配置文件路径 | `./config.json` |
| `CLOSEMASK_ONEAIFW_URL` | OneAIFW 服务 URL | `http://localhost:8844` |
| `CLOSEMASK_LLM_BASE_URL` | LLM API 基础 URL | `https://api.openai.com/v1` |
| `CLOSEMASK_LLM_API_KEY` | LLM API 密钥 | - |
| `CLOSEMASK_REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `CLOSEMASK_LOG_LEVEL` | 日志级别 | `info` |
| `CLOSEMASK_METRICS_ENABLED` | 启用指标 | `true` |

### 配置验证

```bash
# 验证配置文件
closemask -validate-config config.json

# 测试连接
closemask -test-connection config.json
```

## 监控和日志

### Prometheus 监控

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'closemask'
    static_configs:
      - targets: ['closemask:8846']
    metrics_path: '/metrics'
    scrape_interval: 15s
```

### 日志配置

```json
{
  "logging": {
    "level": "info",
    "format": "json",
    "output": "stdout",
    "fields": {
      "service": "closemask",
      "environment": "production"
    }
  }
}
```

### Grafana 仪表板

建议创建以下仪表板：
- 请求速率和延迟
- PII 检测统计
- 错误率
- 资源使用率

## 故障排查

### 常见问题

#### 1. 服务无法启动

```bash
# 检查配置文件语法
closemask -validate-config config.json

# 检查端口占用
netstat -tuln | grep 8846

# 查看详细日志
closemask -config config.json -log-level debug
```

#### 2. OneAIFW 连接失败

```bash
# 测试 OneAIFW 连接
curl http://localhost:8844/health

# 检查防火墙规则
sudo firewall-cmd --list-all

# 查看网络连接
netstat -an | grep 8844
```

#### 3. 性能问题

```bash
# 检查资源使用
top -p $(pgrep closemask)

# 查看 PII 检测延迟
curl http://localhost:8846/metrics | grep pii_mask_duration

# 查看会话数量
curl http://localhost:8846/metrics | grep session_count
```

#### 4. 内存泄漏

```bash
# 监控内存使用
watch -n 1 'ps aux | grep closemask'

# 启用 pprof
CLOSEMASK_PPROF_ENABLED=true closemask -config config.json

# 访问 pprof
go tool pprof http://localhost:6060/debug/pprof/heap
```

### 调试模式

```bash
# 启用详细日志
CLOSEMASK_LOG_LEVEL=debug closemask -config config.json

# 启用 pprof
CLOSEMASK_PPROF_ENABLED=true closemask -config config.json

# 启用追踪
CLOSEMASK_TRACE_ENABLED=true closemask -config config.json
```

## 安全建议

### 网络安全

1. **使用 TLS**：在生产环境中启用 HTTPS
2. **网络隔离**：将 CloseMask 部署在内网
3. **防火墙规则**：限制访问来源

### 数据安全

1. **加密传输**：使用 TLS 加密所有通信
2. **内存保护**：敏感数据不写入磁盘
3. **会话过期**：设置合理的 TTL

### 访问控制

1. **API 认证**：为 API 添加认证机制
2. **IP 白名单**：限制客户端 IP
3. **速率限制**：防止滥用

## 备份和恢复

### 配置备份

```bash
# 备份配置
cp /etc/closemask/config.json /backup/closemask-config.json.$(date +%Y%m%d)

# 恢复配置
cp /backup/closemask-config.json.20250322 /etc/closemask/config.json
```

### 会话数据备份

如果使用 Redis：

```bash
# 备份 Redis 数据
redis-cli BGSAVE

# 导出数据
redis-cli --rdb /backup/redis-dump.rdb
```

## 更新和升级

### 滚动更新

```bash
# 拉取新版本
docker pull closemask:latest

# 停止容器
docker stop closemask

# 启动新容器
docker run -d --name closemask closemask:latest
```

### Kubernetes 更新

```bash
# 更新镜像
kubectl set image deployment/closemask closemask=closemask:1.1.0

# 查看更新状态
kubectl rollout status deployment/closemask

# 回滚
kubectl rollout undo deployment/closemask
```

## 性能调优

### JVM/Go 参数

```bash
# GOGC 调整
export GOGC=100

# GOMAXPROCS 调整
export GOMAXPROCS=4
```

### 连接池配置

```json
{
  "http": {
    "maxIdleConns": 100,
    "maxIdleConnsPerHost": 10,
    "idleConnTimeout": "90s"
  }
}
```

### 会话优化

```json
{
  "session": {
    "ttl": "1h",
    "maxSize": 10000,
    "cleanupInterval": "5m"
  }
}
```

## 参考资料

- [OneAIFW 文档](https://github.com/funstory-ai/aifw)
- [OpenAI API 文档](https://platform.openai.com/docs/api-reference)
- [Prometheus 文档](https://prometheus.io/docs/)
- [Kubernetes 文档](https://kubernetes.io/docs/)
