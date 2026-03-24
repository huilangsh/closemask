# CloseMask Deployment Guide

This document provides detailed deployment instructions for CloseMask, including best practices for different environments.

## Table of Contents

- [System Requirements](#system-requirements)
- [Local Development Environment](#local-development-environment)
- [Production Environment](#production-environment)
- [Containerized Deployment](#containerized-deployment)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Configuration Management](#configuration-management)
- [Monitoring and Logging](#monitoring-and-logging)
- [Troubleshooting](#troubleshooting)

## System Requirements

### Minimum Configuration

- **CPU**: 2 cores
- **Memory**: 4GB
- **Disk**: 20GB
- **OS**: Linux/macOS/Windows

### Recommended Configuration (Production)

- **CPU**: 4 cores
- **Memory**: 8GB
- **Disk**: 50GB SSD
- **Network**: 1Gbps

### Dependency Services

- **OneAIFW**: PII detection engine (required)
- **LLM Provider**: OpenAI-compatible API (required)
- **Redis**: Optional, for distributed session storage

## Local Development Environment

### Method 1: Direct Execution

```bash
# Clone repository
git clone https://github.com/huilangsh/closemask.git
cd closemask

# Build
go build -o closemask ./cmd/proxy

# Run
./closemask
```

### Method 2: Docker Compose

Create `docker-compose.yml`:

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

Start services:

```bash
docker-compose up -d
```

## Production Environment

### System-Level Installation

#### 1. Download Binary

```bash
# Download latest version from GitHub Releases
wget https://github.com/huilangsh/closemask/releases/latest/download/closemask-linux-amd64

# Add execute permission
chmod +x closemask-linux-amd64

# Move to system path
sudo mv closemask-linux-amd64 /usr/local/bin/closemask
```

#### 2. Create Configuration File

```bash
sudo mkdir -p /etc/closemask
sudo nano /etc/closemask/config.json
```

Configuration example:

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

#### 3. Create systemd Service

Create `/etc/systemd/system/closemask.service`:

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

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true

# Resource limits
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

#### 4. Create User and Directories

```bash
# Create dedicated user
sudo useradd -r -s /bin/false closemask

# Create directories
sudo mkdir -p /opt/closemask
sudo chown closemask:closemask /opt/closemask

# Create log directory
sudo mkdir -p /var/log/closemask
sudo chown closemask:closemask /var/log/closemask
```

#### 5. Start Service

```bash
# Reload systemd configuration
sudo systemctl daemon-reload

# Enable auto-start on boot
sudo systemctl enable closemask

# Start service
sudo systemctl start closemask

# Check status
sudo systemctl status closemask

# View logs
sudo journalctl -u closemask -f
```

## Containerized Deployment

### Build Docker Image

#### Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o closemask ./cmd/proxy

# Runtime image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from build stage
COPY --from=builder /app/closemask .

# Copy configuration file
COPY config.json .

EXPOSE 8846

CMD ["./closemask"]
```

#### Build Image

```bash
# Build image
docker build -t closemask:latest .

# Use build args
docker build \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -t closemask:1.0.0 .
```

### Run Container

#### Basic Run

```bash
docker run -d \
  --name closemask \
  -p 8846:8846 \
  -e CLOSEMASK_ONEAIFW_URL=http://host.docker.internal:8844 \
  -e CLOSEMASK_LLM_API_KEY=your-api-key \
  closemask:latest
```

#### With Volume Mounts

```bash
docker run -d \
  --name closemask \
  -p 8846:8846 \
  -v /opt/closemask/config.json:/app/config.json:ro \
  -v /var/log/closemask:/var/log/closemask \
  -e CLOSEMASK_ONEAIFW_URL=http://oneaifw:8844 \
  closemask:latest
```

#### Using Docker Compose

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

## Kubernetes Deployment

### Deployment Configuration

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

### Deploy to Cluster

```bash
# Apply configurations
kubectl apply -f k8s/

# Check deployment status
kubectl get pods -l app=closemask

# View service
kubectl get svc closemask-service

# View logs
kubectl logs -f deployment/closemask

# Scale replicas
kubectl scale deployment closemask --replicas=5
```

## Configuration Management

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CLOSEMASK_CONFIG` | Path to config file | `./config.json` |
| `CLOSEMASK_ONEAIFW_URL` | OneAIFW service URL | `http://localhost:8844` |
| `CLOSEMASK_LLM_BASE_URL` | LLM API base URL | `https://api.openai.com/v1` |
| `CLOSEMASK_LLM_API_KEY` | LLM API key | - |
| `CLOSEMASK_REDIS_ADDR` | Redis address | `localhost:6379` |
| `CLOSEMASK_LOG_LEVEL` | Log level | `info` |
| `CLOSEMASK_METRICS_ENABLED` | Enable metrics | `true` |

### Configuration Validation

```bash
# Validate configuration file
closemask -validate-config config.json

# Test connection
closemask -test-connection config.json
```

## Monitoring and Logging

### Prometheus Monitoring

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'closemask'
    static_configs:
      - targets: ['closemask:8846']
    metrics_path: '/metrics'
    scrape_interval: 15s
```

### Logging Configuration

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

### Grafana Dashboard

Recommended dashboards:
- Request rate and latency
- PII detection statistics
- Error rates
- Resource utilization

## Troubleshooting

### Common Issues

#### 1. Service Won't Start

```bash
# Check configuration file syntax
closemask -validate-config config.json

# Check port usage
netstat -tuln | grep 8846

# View detailed logs
closemask -config config.json -log-level debug
```

#### 2. OneAIFW Connection Failure

```bash
# Test OneAIFW connection
curl http://localhost:8844/health

# Check firewall rules
sudo firewall-cmd --list-all

# Check network connections
netstat -an | grep 8844
```

#### 3. Performance Issues

```bash
# Check resource usage
top -p $(pgrep closemask)

# Check PII detection latency
curl http://localhost:8846/metrics | grep pii_mask_duration

# Check session count
curl http://localhost:8846/metrics | grep session_count
```

#### 4. Memory Leaks

```bash
# Monitor memory usage
watch -n 1 'ps aux | grep closemask'

# Enable pprof
CLOSEMASK_PPROF_ENABLED=true closemask -config config.json

# Access pprof
go tool pprof http://localhost:6060/debug/pprof/heap
```

### Debug Mode

```bash
# Enable verbose logging
CLOSEMASK_LOG_LEVEL=debug closemask -config config.json

# Enable pprof
CLOSEMASK_PPROF_ENABLED=true closemask -config config.json

# Enable tracing
CLOSEMASK_TRACE_ENABLED=true closemask -config config.json
```

## Security Best Practices

### Network Security

1. **Use TLS**: Enable HTTPS in production
2. **Network Isolation**: Deploy CloseMask in private network
3. **Firewall Rules**: Restrict access sources

### Data Security

1. **Encrypted Transmission**: Use TLS for all communication
2. **Memory Protection**: Sensitive data never written to disk
3. **Session Expiration**: Set reasonable TTL

### Access Control

1. **API Authentication**: Add authentication mechanism for API
2. **IP Whitelist**: Restrict client IPs
3. **Rate Limiting**: Prevent abuse

## Backup and Recovery

### Configuration Backup

```bash
# Backup configuration
cp /etc/closemask/config.json /backup/closemask-config.json.$(date +%Y%m%d)

# Restore configuration
cp /backup/closemask-config.json.20250322 /etc/closemask/config.json
```

### Session Data Backup

If using Redis:

```bash
# Backup Redis data
redis-cli BGSAVE

# Export data
redis-cli --rdb /backup/redis-dump.rdb
```

## Update and Upgrade

### Rolling Update

```bash
# Pull new version
docker pull closemask:latest

# Stop container
docker stop closemask

# Start new container
docker run -d --name closemask closemask:latest
```

### Kubernetes Update

```bash
# Update image
kubectl set image deployment/closemask closemask=closemask:1.1.0

# Check update status
kubectl rollout status deployment/closemask

# Rollback
kubectl rollout undo deployment/closemask
```

## Performance Tuning

### JVM/Go Parameters

```bash
# Adjust GOGC
export GOGC=100

# Adjust GOMAXPROCS
export GOMAXPROCS=4
```

### Connection Pool Configuration

```json
{
  "http": {
    "maxIdleConns": 100,
    "maxIdleConnsPerHost": 10,
    "idleConnTimeout": "90s"
  }
}
```

### Session Optimization

```json
{
  "session": {
    "ttl": "1h",
    "maxSize": 10000,
    "cleanupInterval": "5m"
  }
}
```

## References

- [OneAIFW Documentation](https://github.com/funstory-ai/aifw)
- [OpenAI API Documentation](https://platform.openai.com/docs/api-reference)
- [Prometheus Documentation](https://prometheus.io/docs/)
- [Kubernetes Documentation](https://kubernetes.io/docs/)
