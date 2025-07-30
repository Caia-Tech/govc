# govc Production Deployment Guide

This guide covers deploying govc in production environments with best practices for security, performance, and reliability.

## Table of Contents

- [System Requirements](#system-requirements)
- [Pre-deployment Checklist](#pre-deployment-checklist)
- [Installation Methods](#installation-methods)
- [Configuration](#configuration)
- [Security Hardening](#security-hardening)
- [Performance Tuning](#performance-tuning)
- [High Availability](#high-availability)
- [Monitoring & Observability](#monitoring--observability)
- [Backup & Recovery](#backup--recovery)
- [Troubleshooting](#troubleshooting)
- [Maintenance](#maintenance)

## System Requirements

### Minimum Requirements

- **CPU**: 2 cores (2.0 GHz+)
- **Memory**: 4GB RAM
- **Storage**: 20GB available space
- **OS**: Linux (Ubuntu 20.04+, RHEL 8+, Debian 11+)
- **Go**: 1.21+ (for building from source)

### Recommended Requirements

- **CPU**: 4+ cores (3.0 GHz+)
- **Memory**: 16GB+ RAM
- **Storage**: 100GB+ SSD storage
- **Network**: 1Gbps+ network interface
- **OS**: Ubuntu 22.04 LTS or RHEL 9

### Scaling Guidelines

| Users | Repositories | CPU | RAM | Storage |
|-------|--------------|-----|-----|---------|
| < 100 | < 1,000 | 4 cores | 8GB | 100GB |
| < 500 | < 5,000 | 8 cores | 32GB | 500GB |
| < 1,000 | < 10,000 | 16 cores | 64GB | 1TB |
| 1,000+ | 10,000+ | 32+ cores | 128GB+ | 2TB+ |

## Pre-deployment Checklist

- [ ] Review system requirements
- [ ] Configure firewall rules
- [ ] Set up SSL certificates
- [ ] Create database backups directory
- [ ] Configure monitoring infrastructure
- [ ] Set up log aggregation
- [ ] Review security policies
- [ ] Plan maintenance windows
- [ ] Document emergency procedures

## Installation Methods

### 1. Binary Installation

Download the latest release:

```bash
# Download binary
wget https://github.com/caiatech/govc/releases/latest/download/govc-linux-amd64.tar.gz

# Extract
tar xzf govc-linux-amd64.tar.gz

# Install
sudo mv govc /usr/local/bin/
sudo chmod +x /usr/local/bin/govc

# Create system user
sudo useradd -r -s /bin/false govc
```

### 2. Docker Installation

```bash
# Pull official image
docker pull ghcr.io/caiatech/govc:latest

# Run with Docker
docker run -d \
  --name govc \
  -p 8080:8080 \
  -v /data/govc:/data \
  -v /etc/govc:/etc/govc \
  --restart unless-stopped \
  ghcr.io/caiatech/govc:latest
```

### 3. Kubernetes Deployment

```yaml
# govc-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: govc
  namespace: govc
spec:
  replicas: 3
  selector:
    matchLabels:
      app: govc
  template:
    metadata:
      labels:
        app: govc
    spec:
      containers:
      - name: govc
        image: ghcr.io/caiatech/govc:latest
        ports:
        - containerPort: 8080
        env:
        - name: GOVC_CONFIG
          value: /etc/govc/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/govc
        - name: data
          mountPath: /data
        resources:
          requests:
            memory: "4Gi"
            cpu: "2"
          limits:
            memory: "8Gi"
            cpu: "4"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: govc-config
      - name: data
        persistentVolumeClaim:
          claimName: govc-data
```

### 4. Build from Source

```bash
# Clone repository
git clone https://github.com/caiatech/govc.git
cd govc

# Build
go build -o govc-server ./cmd/server

# Install
sudo cp govc-server /usr/local/bin/
sudo chmod +x /usr/local/bin/govc-server
```

## Configuration

### 1. Create Configuration File

```bash
sudo mkdir -p /etc/govc
sudo cat > /etc/govc/config.yaml << EOF
# /etc/govc/config.yaml
server:
  port: 8080
  host: 0.0.0.0
  read_timeout: 30s
  write_timeout: 30s
  request_timeout: 25s
  max_request_size: 10485760  # 10MB
  enable_cors: true
  cors_origins:
    - https://app.example.com
    - https://ci.example.com

auth:
  jwt:
    secret: ${JWT_SECRET}  # Use strong secret
    issuer: govc-prod
    ttl: 24h
  api_key:
    enabled: true
    header: X-API-Key
  session:
    timeout: 30m
    max_concurrent: 1000

pool:
  max_repositories: 5000
  max_idle_time: 30m
  cleanup_interval: 5m
  preload_popular: true
  popular_threshold: 10

storage:
  data_dir: /data/govc
  temp_dir: /tmp/govc
  max_file_size: 104857600  # 100MB
  enable_compression: true

logging:
  level: INFO
  format: json
  output: stdout
  file:
    enabled: true
    path: /var/log/govc/server.log
    max_size: 100  # MB
    max_backups: 10
    max_age: 30    # days

metrics:
  enabled: true
  path: /metrics
  include_system: true
  include_go: true

security:
  tls:
    enabled: true
    cert_file: /etc/govc/certs/server.crt
    key_file: /etc/govc/certs/server.key
  rate_limit:
    enabled: true
    requests_per_minute: 100
    burst: 200
  allowed_hosts:
    - govc.example.com
    - api.govc.example.com

database:
  type: postgres  # or mysql
  host: localhost
  port: 5432
  name: govc
  user: govc
  password: ${DB_PASSWORD}
  ssl_mode: require
  max_connections: 100
  max_idle: 10
  connection_lifetime: 1h
EOF
```

### 2. Environment Variables

```bash
# Create environment file
sudo cat > /etc/govc/govc.env << EOF
# JWT Secret (generate with: openssl rand -base64 32)
JWT_SECRET=your-super-secret-jwt-key

# Database Password
DB_PASSWORD=your-secure-db-password

# Admin User
GOVC_ADMIN_USERNAME=admin
GOVC_ADMIN_PASSWORD=change-this-password
GOVC_ADMIN_EMAIL=admin@example.com

# SMTP Configuration
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=notifications@example.com
SMTP_PASSWORD=smtp-password

# S3 Backup (optional)
S3_BUCKET=govc-backups
S3_REGION=us-east-1
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key
EOF

# Secure the file
sudo chmod 600 /etc/govc/govc.env
sudo chown govc:govc /etc/govc/govc.env
```

### 3. Systemd Service

```bash
# Create service file
sudo cat > /etc/systemd/system/govc.service << EOF
[Unit]
Description=govc Git Server
Documentation=https://github.com/caiatech/govc
After=network.target

[Service]
Type=simple
User=govc
Group=govc
EnvironmentFile=/etc/govc/govc.env
ExecStart=/usr/local/bin/govc-server serve --config /etc/govc/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=govc

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/data/govc /var/log/govc

# Resource Limits
LimitNOFILE=65535
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable govc
sudo systemctl start govc
```

## Security Hardening

### 1. SSL/TLS Configuration

```bash
# Generate self-signed certificate (for testing)
sudo mkdir -p /etc/govc/certs
sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /etc/govc/certs/server.key \
  -out /etc/govc/certs/server.crt \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=govc.example.com"

# For production, use Let's Encrypt
sudo certbot certonly --standalone -d govc.example.com
```

### 2. Nginx Reverse Proxy

```nginx
# /etc/nginx/sites-available/govc
upstream govc_backend {
    server 127.0.0.1:8080 max_fails=3 fail_timeout=30s;
    keepalive 32;
}

server {
    listen 80;
    server_name govc.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name govc.example.com;

    ssl_certificate /etc/letsencrypt/live/govc.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/govc.example.com/privkey.pem;
    
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    
    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    
    # Rate limiting
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
    limit_req zone=api burst=20 nodelay;
    
    # Proxy settings
    location / {
        proxy_pass http://govc_backend;
        proxy_http_version 1.1;
        
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        
        # WebSocket support for SSE
        proxy_set_header Connection '';
        proxy_buffering off;
        proxy_cache off;
    }
    
    # Metrics endpoint (restrict access)
    location /metrics {
        allow 10.0.0.0/8;  # Internal network only
        deny all;
        proxy_pass http://govc_backend;
    }
}
```

### 3. Firewall Configuration

```bash
# UFW (Ubuntu)
sudo ufw allow 22/tcp     # SSH
sudo ufw allow 80/tcp     # HTTP
sudo ufw allow 443/tcp    # HTTPS
sudo ufw enable

# iptables
sudo iptables -A INPUT -p tcp --dport 22 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 80 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 443 -j ACCEPT
sudo iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
sudo iptables -A INPUT -i lo -j ACCEPT
sudo iptables -P INPUT DROP
```

### 4. Security Hardening Checklist

- [ ] Change default passwords
- [ ] Enable TLS/SSL
- [ ] Configure firewall rules
- [ ] Disable unnecessary services
- [ ] Enable SELinux/AppArmor
- [ ] Regular security updates
- [ ] Configure fail2ban
- [ ] Enable audit logging
- [ ] Implement backup encryption

## Performance Tuning

### 1. System Optimization

```bash
# Kernel parameters
sudo cat >> /etc/sysctl.conf << EOF
# Network optimizations
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.ip_local_port_range = 1024 65535
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 30

# File system
fs.file-max = 2097152
fs.nr_open = 1048576

# Memory
vm.swappiness = 10
vm.dirty_ratio = 15
vm.dirty_background_ratio = 5
EOF

sudo sysctl -p
```

### 2. govc Performance Configuration

```yaml
# Performance-optimized settings
pool:
  max_repositories: 10000
  max_idle_time: 60m
  cleanup_interval: 10m
  preload_popular: true
  popular_threshold: 5
  
  # Connection pooling
  max_connections_per_repo: 100
  connection_timeout: 5s
  
  # Caching
  cache:
    enabled: true
    size: 1GB
    ttl: 1h
    
performance:
  # Concurrency
  max_concurrent_requests: 1000
  worker_pool_size: 100
  
  # Memory
  gc_percent: 100  # Default GOGC
  max_memory: 8GB
  
  # CPU
  gomaxprocs: 0  # Use all cores
```

### 3. Database Optimization

```sql
-- PostgreSQL optimizations
ALTER SYSTEM SET shared_buffers = '4GB';
ALTER SYSTEM SET effective_cache_size = '12GB';
ALTER SYSTEM SET maintenance_work_mem = '1GB';
ALTER SYSTEM SET checkpoint_completion_target = 0.9;
ALTER SYSTEM SET wal_buffers = '16MB';
ALTER SYSTEM SET default_statistics_target = 100;
ALTER SYSTEM SET random_page_cost = 1.1;
ALTER SYSTEM SET effective_io_concurrency = 200;
ALTER SYSTEM SET work_mem = '32MB';
ALTER SYSTEM SET min_wal_size = '1GB';
ALTER SYSTEM SET max_wal_size = '4GB';

-- Create indexes
CREATE INDEX idx_repos_updated_at ON repositories(updated_at);
CREATE INDEX idx_commits_author ON commits(author_email);
CREATE INDEX idx_branches_repo_id ON branches(repository_id);
```

## High Availability

### 1. Load Balancer Configuration (HAProxy)

```bash
# /etc/haproxy/haproxy.cfg
global
    maxconn 4096
    log /dev/log local0
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

defaults
    mode http
    log global
    option httplog
    option dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000
    errorfile 400 /etc/haproxy/errors/400.http
    errorfile 403 /etc/haproxy/errors/403.http
    errorfile 408 /etc/haproxy/errors/408.http
    errorfile 500 /etc/haproxy/errors/500.http
    errorfile 502 /etc/haproxy/errors/502.http
    errorfile 503 /etc/haproxy/errors/503.http
    errorfile 504 /etc/haproxy/errors/504.http

frontend govc_frontend
    bind *:443 ssl crt /etc/ssl/certs/govc.pem
    redirect scheme https if !{ ssl_fc }
    
    # Rate limiting
    stick-table type ip size 100k expire 30s store http_req_rate(10s)
    http-request track-sc0 src
    http-request deny if { sc_http_req_rate(0) gt 20 }
    
    default_backend govc_backend

backend govc_backend
    balance roundrobin
    option httpchk GET /health/ready
    
    server govc1 10.0.1.10:8080 check
    server govc2 10.0.1.11:8080 check
    server govc3 10.0.1.12:8080 check
```

### 2. Database Replication

```bash
# Primary database
postgresql://primary.db.example.com:5432/govc

# Read replicas
postgresql://replica1.db.example.com:5432/govc
postgresql://replica2.db.example.com:5432/govc

# Connection pooling with PgBouncer
[databases]
govc = host=primary.db.example.com port=5432 dbname=govc
govc_ro = host=replica1.db.example.com port=5432 dbname=govc

[pgbouncer]
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 25
```

### 3. Redis for Caching

```bash
# Install Redis
sudo apt-get install redis-server

# Configure Redis
sudo cat > /etc/redis/redis.conf << EOF
bind 127.0.0.1
protected-mode yes
port 6379
tcp-backlog 511
timeout 0
tcp-keepalive 300
daemonize yes
supervised systemd
pidfile /var/run/redis/redis-server.pid
loglevel notice
logfile /var/log/redis/redis-server.log
databases 16
save 900 1
save 300 10
save 60 10000
stop-writes-on-bgsave-error yes
rdbcompression yes
rdbchecksum yes
dbfilename dump.rdb
dir /var/lib/redis
maxmemory 2gb
maxmemory-policy allkeys-lru
EOF
```

## Monitoring & Observability

### 1. Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'govc'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 10s
```

### 2. Grafana Dashboard

Import the govc dashboard:
- Dashboard ID: 12345 (example)
- Or use the provided `grafana-dashboard.json`

Key metrics to monitor:
- Request rate and latency
- Error rate
- Repository count
- Active connections
- Memory usage
- CPU usage
- Disk I/O

### 3. Alerting Rules

```yaml
# prometheus-alerts.yml
groups:
  - name: govc
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: High error rate detected
          
      - alert: HighMemoryUsage
        expr: process_resident_memory_bytes / 1024 / 1024 / 1024 > 7
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High memory usage (>7GB)
          
      - alert: HighResponseTime
        expr: histogram_quantile(0.95, http_request_duration_seconds_bucket) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High response time (>1s at p95)
```

### 4. Logging

Configure centralized logging with ELK stack:

```bash
# Filebeat configuration
filebeat.inputs:
- type: log
  enabled: true
  paths:
    - /var/log/govc/*.log
  json.keys_under_root: true
  json.add_error_key: true

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
  index: "govc-%{+yyyy.MM.dd}"
```

## Backup & Recovery

### 1. Automated Backups

```bash
#!/bin/bash
# /usr/local/bin/govc-backup.sh

set -e

BACKUP_DIR="/backup/govc"
DATE=$(date +%Y%m%d_%H%M%S)
RETENTION_DAYS=30

# Create backup directory
mkdir -p $BACKUP_DIR

# Backup database
pg_dump -h localhost -U govc -d govc | gzip > $BACKUP_DIR/db_$DATE.sql.gz

# Backup repositories
tar czf $BACKUP_DIR/repos_$DATE.tar.gz /data/govc/repos/

# Backup configuration
tar czf $BACKUP_DIR/config_$DATE.tar.gz /etc/govc/

# Upload to S3 (optional)
aws s3 cp $BACKUP_DIR/db_$DATE.sql.gz s3://govc-backups/db/
aws s3 cp $BACKUP_DIR/repos_$DATE.tar.gz s3://govc-backups/repos/
aws s3 cp $BACKUP_DIR/config_$DATE.tar.gz s3://govc-backups/config/

# Clean old backups
find $BACKUP_DIR -name "*.gz" -mtime +$RETENTION_DAYS -delete

# Log
echo "Backup completed: $DATE" >> /var/log/govc/backup.log
```

### 2. Backup Schedule

```bash
# Add to crontab
0 2 * * * /usr/local/bin/govc-backup.sh
```

### 3. Recovery Procedure

```bash
# Stop service
sudo systemctl stop govc

# Restore database
gunzip < /backup/govc/db_20240115_020000.sql.gz | psql -h localhost -U govc -d govc

# Restore repositories
tar xzf /backup/govc/repos_20240115_020000.tar.gz -C /

# Restore configuration
tar xzf /backup/govc/config_20240115_020000.tar.gz -C /

# Start service
sudo systemctl start govc
```

## Troubleshooting

### Common Issues

#### 1. Service Won't Start

```bash
# Check logs
sudo journalctl -u govc -n 100

# Check configuration
govc-server validate --config /etc/govc/config.yaml

# Check permissions
ls -la /data/govc/
ls -la /etc/govc/
```

#### 2. High Memory Usage

```bash
# Check memory usage
ps aux | grep govc
free -h

# Force garbage collection
curl -X POST http://localhost:8080/debug/gc

# Check repository pool
curl http://localhost:8080/debug/pool
```

#### 3. Slow Performance

```bash
# Check database performance
psql -U govc -c "SELECT * FROM pg_stat_activity;"

# Check disk I/O
iostat -x 1

# Check network
netstat -an | grep :8080
```

### Debug Endpoints

Enable debug endpoints in development:

```yaml
debug:
  enabled: true
  path: /debug
  pprof: true
```

Access debug information:
- `/debug/vars` - Application variables
- `/debug/pprof` - CPU and memory profiling
- `/debug/pool` - Repository pool status
- `/debug/gc` - Force garbage collection

## Maintenance

### 1. Regular Tasks

#### Daily
- Monitor error logs
- Check disk space
- Verify backups

#### Weekly
- Review performance metrics
- Check for security updates
- Analyze slow queries

#### Monthly
- Update dependencies
- Review access logs
- Performance tuning
- Security audit

### 2. Upgrade Procedure

```bash
# 1. Backup everything
/usr/local/bin/govc-backup.sh

# 2. Download new version
wget https://github.com/caiatech/govc/releases/latest/download/govc-linux-amd64.tar.gz

# 3. Stop service
sudo systemctl stop govc

# 4. Replace binary
sudo mv /usr/local/bin/govc-server /usr/local/bin/govc-server.old
sudo tar xzf govc-linux-amd64.tar.gz -C /usr/local/bin/

# 5. Run migrations (if any)
govc-server migrate --config /etc/govc/config.yaml

# 6. Start service
sudo systemctl start govc

# 7. Verify
curl https://govc.example.com/health
```

### 3. Scaling Up

When you need to scale:

1. **Vertical Scaling**
   - Increase CPU/RAM
   - Optimize configuration
   - Upgrade storage to SSD

2. **Horizontal Scaling**
   - Add more application servers
   - Implement load balancing
   - Use read replicas
   - Add caching layer

3. **Database Scaling**
   - Partition large tables
   - Add read replicas
   - Consider sharding
   - Optimize queries

## Production Checklist

Before going live:

- [ ] SSL/TLS configured
- [ ] Firewall rules set
- [ ] Monitoring configured
- [ ] Alerting configured
- [ ] Backups scheduled
- [ ] Security hardened
- [ ] Performance tested
- [ ] Documentation complete
- [ ] Runbooks created
- [ ] On-call rotation set
- [ ] Disaster recovery tested

## Support

- Documentation: https://docs.govc.io
- Issues: https://github.com/caiatech/govc/issues
- Community: https://discord.gg/govc
- Commercial Support: support@govc.io