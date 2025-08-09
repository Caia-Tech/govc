# govc Dashboard Monitoring

This directory contains the complete monitoring and alerting setup for the govc dashboard.

## Overview

The monitoring stack includes:
- **Prometheus**: Metrics collection and alerting
- **Grafana**: Visualization and dashboards
- **Alertmanager**: Alert routing and notification
- **Node Exporter**: System metrics
- **Loki**: Log aggregation (optional)

## Quick Start

1. **Start the monitoring stack:**
   ```bash
   cd monitoring
   docker-compose -f docker-compose.monitoring.yml up -d
   ```

2. **Access the services:**
   - Grafana: http://localhost:3000 (admin/admin)
   - Prometheus: http://localhost:9090
   - Alertmanager: http://localhost:9093

3. **Start govc with metrics enabled:**
   ```bash
   # Ensure your govc server has Prometheus metrics endpoint
   ./govc-server --config config.yaml --enable-metrics
   ```

## Configuration

### Environment Variables

Create a `.env` file in the monitoring directory:

```bash
# Grafana
GRAFANA_USER=admin
GRAFANA_PASSWORD=your-secure-password

# Alertmanager
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK
PAGERDUTY_SERVICE_KEY=your-pagerduty-integration-key

# Email (for alertmanager.yml)
SMTP_PASSWORD=your-email-password
```

### Customization

1. **Update targets** in `prometheus.yml` to match your deployment
2. **Configure alerting** in `alertmanager.yml` with your notification channels
3. **Customize alerts** in `prometheus-alerts.yml` for your SLOs

## Metrics Collected

### Dashboard Metrics

- **Request latency**: `govc_dashboard_request_duration_seconds`
- **Request rate**: `govc_dashboard_requests_total`
- **Error rate**: `govc_dashboard_errors_total`
- **WebSocket connections**: `govc_dashboard_websocket_connections`
- **Memory usage**: `govc_dashboard_memory_usage_bytes`
- **Cache performance**: `govc_dashboard_cache_hits_total`

### Business Metrics

- **Active users**: `govc_dashboard_active_users`
- **Repository views**: `govc_dashboard_repository_views_total`
- **Actions performed**: `govc_dashboard_actions_performed_total`

## Alerting Rules

### Critical Alerts
- **DashboardDown**: Dashboard is completely unavailable
- **DashboardCriticalErrorRate**: Error rate > 10%
- **DashboardVeryHighLatency**: 95th percentile > 2s

### Warning Alerts
- **DashboardHighLatency**: 95th percentile > 500ms
- **DashboardHighErrorRate**: Error rate > 5%
- **DashboardHighMemoryUsage**: Memory > 500MB
- **WebSocketConnectionSpike**: > 1000 connections

### SLO Alerts
- **DashboardSLOLatencyBreach**: < 95% requests under 500ms
- **DashboardSLOAvailabilityBreach**: < 99% uptime

## Dashboard Features

The Grafana dashboard includes:

1. **Performance Overview**
   - Request latency percentiles
   - Request rate by endpoint
   - Error rate trending

2. **Resource Monitoring**
   - Memory usage
   - Goroutine count
   - Cache hit rates

3. **User Activity**
   - Active WebSocket connections
   - User actions breakdown
   - Repository access patterns

4. **Alerts Status**
   - Current firing alerts
   - Alert history

## Runbooks

### High Latency Response

1. Check Grafana dashboard for affected endpoints
2. Review error logs in application
3. Check resource usage (CPU, Memory)
4. Verify database/storage performance
5. Scale horizontally if needed

### Dashboard Down

1. Check if process is running: `ps aux | grep govc-server`
2. Check logs: `tail -f /var/log/govc/server.log`
3. Verify configuration files
4. Check database connectivity
5. Restart service if necessary

### Memory Issues

1. Check for memory leaks in dashboard code
2. Review WebSocket connection management
3. Analyze garbage collection patterns
4. Consider increasing memory limits
5. Restart service if memory leak confirmed

## Production Deployment

### Security Considerations

1. **Enable authentication** in Grafana
2. **Use HTTPS** for all monitoring services
3. **Restrict network access** to monitoring stack
4. **Rotate credentials** regularly
5. **Enable audit logging**

### High Availability

1. **Run multiple Prometheus instances** with federation
2. **Use external storage** for Grafana dashboards
3. **Deploy Alertmanager cluster** for reliability
4. **Backup configurations** regularly

### Scaling

1. **Use Prometheus sharding** for large deployments
2. **Consider Thanos** for long-term storage
3. **Deploy regional monitoring** for global services
4. **Use recording rules** for expensive queries

## Troubleshooting

### Common Issues

1. **Metrics not appearing**
   - Check Prometheus targets: http://localhost:9090/targets
   - Verify govc metrics endpoint: `curl http://localhost:8080/metrics`
   - Check firewall settings

2. **Alerts not firing**
   - Verify alert rules syntax
   - Check Alertmanager configuration
   - Test notification channels

3. **Dashboard not loading**
   - Check Grafana logs: `docker logs govc-grafana`
   - Verify datasource connectivity
   - Clear browser cache

### Debugging Commands

```bash
# Check Prometheus configuration
docker exec govc-prometheus promtool check config /etc/prometheus/prometheus.yml

# Test alert rules
docker exec govc-prometheus promtool check rules /etc/prometheus/alerts.yml

# Check Alertmanager configuration
docker exec govc-alertmanager amtool check-config /etc/alertmanager/alertmanager.yml

# View Prometheus targets
curl http://localhost:9090/api/v1/targets

# Check alert status
curl http://localhost:9090/api/v1/alerts
```

## Maintenance

### Regular Tasks

1. **Monitor disk usage** for Prometheus data
2. **Review and tune** alert thresholds
3. **Update dashboards** based on user feedback
4. **Test alert channels** monthly
5. **Review and archive** old metrics data

### Updates

1. **Keep monitoring stack updated** with latest versions
2. **Review new Prometheus features** quarterly
3. **Update alert rules** when dashboard changes
4. **Migrate to new Grafana features** as available

## Support

For monitoring-related issues:
1. Check this README and troubleshooting section
2. Review monitoring stack logs
3. Consult Prometheus/Grafana documentation
4. Contact the platform team