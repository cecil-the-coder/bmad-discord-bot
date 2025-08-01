# Kubernetes Deployment Guide

This guide covers deploying the BMAD Discord Bot to a Kubernetes cluster with MySQL database support.

## Prerequisites

- Kubernetes cluster (v1.20+)
- kubectl configured for your cluster
- Docker registry access (GitHub Container Registry)
- MySQL database (external or in-cluster)
- Discord bot token

## Architecture Overview

The deployment includes:
- **Discord Bot**: Main application container
- **MySQL Database**: External database service for persistence
- **Security**: RBAC, NetworkPolicies, SecurityContext
- **Monitoring**: Health checks, resource limits, HPA
- **CI/CD**: GitHub Actions for automated builds and deployments

## Database Support

The bot supports dual database configuration:

### MySQL (Recommended for Production)
- **External MySQL**: Configure `mysql-service` to point to your MySQL instance
- **In-cluster MySQL**: Use the commented MySQL deployment in `persistentvolume.yaml`
- **Database Migration**: Automatic schema migration on startup
- **Configuration**: All stored in database with hot-reload capability

### SQLite (Fallback/Development)
- **Local Storage**: Uses PersistentVolume for data persistence
- **Development**: Suitable for development and testing
- **Migration**: Automatic migration from SQLite to MySQL supported

## Quick Start

### 1. Prepare Secrets

Create your Discord bot token and MySQL credentials:

```bash
# Create namespace first
kubectl apply -f k8s/namespace.yaml

# Create secrets with actual values
kubectl create secret generic bmad-bot-secrets \
  --namespace=bmad-bot \
  --from-literal=BOT_TOKEN="your_discord_bot_token_here" \
  --from-literal=MYSQL_PASSWORD="your_mysql_password_here"
```

### 2. Configure MySQL Connection

Edit `k8s/configmap.yaml` to update MySQL connection details:
- `MYSQL_HOST`: Your MySQL server hostname
- `MYSQL_PORT`: MySQL port (default: 3306)
- `MYSQL_DATABASE`: Database name
- `MYSQL_USERNAME`: MySQL username

Or update `k8s/persistentvolume.yaml` ExternalName service:
```yaml
spec:
  externalName: your-mysql-host.example.com
```

### 3. Deploy to Kubernetes

Using Kustomize (recommended):
```bash
kubectl apply -k k8s/
```

Using individual manifests:
```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/serviceaccount.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/secret.yaml
kubectl apply -f k8s/persistentvolume.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/networkpolicy.yaml
kubectl apply -f k8s/hpa.yaml
```

### 4. Verify Deployment

```bash
# Check deployment status
kubectl get all -n bmad-bot

# Check pod logs
kubectl logs -f deployment/bmad-discord-bot -n bmad-bot

# Check health status
kubectl get pods -n bmad-bot
```

## Configuration Management

### Environment Variables

**Required Variables** (stored in Secret):
- `BOT_TOKEN`: Discord bot token
- `MYSQL_PASSWORD`: MySQL database password

**Configuration Variables** (stored in ConfigMap):
- `DATABASE_TYPE`: Set to "mysql"
- `MYSQL_HOST`, `MYSQL_PORT`, `MYSQL_DATABASE`, `MYSQL_USERNAME`
- `AI_PROVIDER`: Set to "ollama" (only supported provider)
- All Ollama configuration variables

### Dynamic Configuration

The bot supports database-backed configuration with hot-reload:
1. Initial deployment migrates environment variables to database
2. Runtime configuration changes through database without restart
3. Configuration reload interval: 1 minute (configurable)

## Security Configuration

### Container Security
- **Non-root execution**: Runs as user ID 1000
- **Read-only filesystem**: Except for logs and data volumes
- **No privileges**: All capabilities dropped
- **Security context**: Comprehensive security restrictions

### Network Security
- **NetworkPolicy**: Restricts ingress/egress traffic
- **Allowed outbound**: DNS, HTTPS (Discord/Ollama), MySQL
- **No inbound**: Bot doesn't accept incoming connections

### RBAC
- **ServiceAccount**: Minimal permissions
- **Role**: Limited to event creation only
- **RoleBinding**: Namespace-scoped permissions

## Resource Management

### Resource Limits
Based on Docker Compose specifications:
- **Memory**: 512Mi limit, 256Mi request
- **CPU**: 500m limit, 250m request
- **Storage**: 1Gi PVC for data persistence

### Horizontal Pod Autoscaler
- **Min replicas**: 1
- **Max replicas**: 3
- **CPU target**: 70% utilization
- **Memory target**: 80% utilization

## Health Monitoring

### Probes Configuration
- **Liveness**: Health check every 30s
- **Readiness**: Health check every 10s
- **Startup**: Health check every 5s (max 60s)

### Health Check Command
```bash
/app/main --health-check
```

## Troubleshooting

### Common Issues

**Pod CrashLoopBackOff**:
```bash
# Check pod logs
kubectl logs deployment/bmad-discord-bot -n bmad-bot

# Check events
kubectl get events -n bmad-bot --sort-by='.lastTimestamp'
```

**Database Connection Issues**:
```bash
# Test MySQL connectivity
kubectl run mysql-test --rm -i --tty --image=mysql:8.0 -- mysql -h mysql-service -u bmad_user -p

# Check service endpoints
kubectl get endpoints mysql-service -n bmad-bot
```

**Discord API Issues**:
```bash
# Verify bot token
kubectl get secret bmad-bot-secrets -n bmad-bot -o yaml

# Check Discord API connectivity
kubectl exec deployment/bmad-discord-bot -n bmad-bot -- nslookup discord.com
```

### Debugging Commands

```bash
# Get detailed pod information
kubectl describe pod -l app=bmad-discord-bot -n bmad-bot

# Access pod shell (if needed)
kubectl exec -it deployment/bmad-discord-bot -n bmad-bot -- /bin/sh

# View resource usage
kubectl top pod -n bmad-bot

# Check HPA status
kubectl get hpa -n bmad-bot
```

## Migration from SQLite to MySQL

If migrating from an existing SQLite deployment:

1. **Backup existing data**:
   ```bash
   kubectl cp bmad-bot/pod-name:/app/data/bot_state.db ./backup.db
   ```

2. **Deploy MySQL configuration**:
   ```bash
   kubectl apply -k k8s/
   ```

3. **Bot automatically migrates data on startup**

## Rollback Procedures

### Rolling Back Deployment
```bash
# Check rollout history
kubectl rollout history deployment/bmad-discord-bot -n bmad-bot

# Rollback to previous version
kubectl rollout undo deployment/bmad-discord-bot -n bmad-bot

# Rollback to specific revision
kubectl rollout undo deployment/bmad-discord-bot --to-revision=2 -n bmad-bot
```

### Emergency Procedures
```bash
# Scale down immediately
kubectl scale deployment/bmad-discord-bot --replicas=0 -n bmad-bot

# Scale back up
kubectl scale deployment/bmad-discord-bot --replicas=1 -n bmad-bot
```

## Update Strategies

### Image Updates
```bash
# Update image tag in kustomization.yaml
kubectl patch deployment bmad-discord-bot -n bmad-bot -p '{"spec":{"template":{"spec":{"containers":[{"name":"bmad-discord-bot","image":"ghcr.io/cecil-the-coder/bmad-discord-bot/bmad-discord-bot:new-tag"}]}}}}'

# Monitor rollout
kubectl rollout status deployment/bmad-discord-bot -n bmad-bot
```

### Configuration Updates
```bash
# Update ConfigMap
kubectl patch configmap bmad-bot-config -n bmad-bot --patch='{"data":{"NEW_KEY":"new_value"}}'

# Restart deployment to pick up changes
kubectl rollout restart deployment/bmad-discord-bot -n bmad-bot
```

## Performance Tuning

### Resource Optimization
- Monitor actual resource usage with `kubectl top`
- Adjust resource requests/limits based on actual usage
- Configure HPA thresholds based on load patterns

### Database Optimization
- Monitor MySQL performance and connection pooling
- Consider read replicas for high-load scenarios
- Optimize database queries and indexes

## Security Best Practices

1. **Secret Management**: Use external secret management (Vault, External Secrets)
2. **Network Policies**: Regularly review and update network policies
3. **Image Security**: Use vulnerability scanning in CI/CD pipeline
4. **RBAC**: Follow principle of least privilege
5. **Updates**: Keep Kubernetes cluster and container images updated