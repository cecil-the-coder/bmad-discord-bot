# MySQL Deployment Guide

This guide covers deploying the BMAD Discord Bot with MySQL database support for cloud-native environments.

## Overview

The bot supports two database configurations:
- **SQLite** (default): File-based database for single-instance deployments
- **MySQL**: External database for cloud-native, scalable deployments

## MySQL Configuration

### Environment Variables

Configure the following environment variables for MySQL deployment:

```bash
# Database type selection
DATABASE_TYPE=mysql

# MySQL connection settings
MYSQL_HOST=your-mysql-host
MYSQL_PORT=3306
MYSQL_DATABASE=bmad_bot
MYSQL_USERNAME=your-username
MYSQL_PASSWORD=your-secure-password
MYSQL_TIMEOUT=30s
```

### Docker Compose Deployment

1. **Copy and modify docker-compose.yml**:
   ```bash
   cp docker-compose.yml docker-compose.mysql.yml
   ```

2. **Uncomment MySQL service in docker-compose.mysql.yml**:
   - Uncomment the entire `mysql:` service block
   - Uncomment the `depends_on:` section for bmad-bot
   - Uncomment the `volumes:` section at the bottom

3. **Create MySQL initialization directory**:
   ```bash
   mkdir -p mysql/init
   ```

4. **Deploy with MySQL**:
   ```bash
   docker-compose -f docker-compose.mysql.yml up -d
   ```

### Kubernetes Deployment

#### MySQL Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mysql
spec:
  selector:
    matchLabels:
      app: mysql
  template:
    metadata:
      labels:
        app: mysql
    spec:
      containers:
      - name: mysql
        image: mysql:8.0
        env:
        - name: MYSQL_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: mysql-secret
              key: root-password
        - name: MYSQL_DATABASE
          value: "bmad_bot"
        - name: MYSQL_USER
          value: "bmad_user"
        - name: MYSQL_PASSWORD
          valueFrom:
            secretKeyRef:
              name: mysql-secret
              key: user-password
        ports:
        - containerPort: 3306
        volumeMounts:
        - name: mysql-storage
          mountPath: /var/lib/mysql
      volumes:
      - name: mysql-storage
        persistentVolumeClaim:
          claimName: mysql-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: mysql
spec:
  selector:
    app: mysql
  ports:
  - port: 3306
    targetPort: 3306
```

#### Bot Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bmad-discord-bot
spec:
  replicas: 1
  selector:
    matchLabels:
      app: bmad-discord-bot
  template:
    metadata:
      labels:
        app: bmad-discord-bot
    spec:
      containers:
      - name: bmad-discord-bot
        image: bmad-knowledge-bot:latest
        env:
        - name: DATABASE_TYPE
          value: "mysql"
        - name: MYSQL_HOST
          value: "mysql"
        - name: MYSQL_PORT
          value: "3306"
        - name: MYSQL_DATABASE
          value: "bmad_bot"
        - name: MYSQL_USERNAME
          value: "bmad_user"
        - name: MYSQL_PASSWORD
          valueFrom:
            secretKeyRef:
              name: mysql-secret
              key: user-password
        - name: BOT_TOKEN
          valueFrom:
            secretKeyRef:
              name: discord-secret
              key: bot-token
        # Add other required environment variables...
```

### Cloud SQL (Google Cloud Platform)

For Google Cloud deployment with Cloud SQL:

```bash
# Set up Cloud SQL proxy
export DATABASE_TYPE=mysql
export MYSQL_HOST=127.0.0.1
export MYSQL_PORT=3306
export MYSQL_DATABASE=bmad_bot
export MYSQL_USERNAME=your-username
export MYSQL_PASSWORD=your-password

# Start Cloud SQL proxy
cloud_sql_proxy -instances=your-project:region:instance-name=tcp:3306
```

### Amazon RDS (AWS)

For AWS deployment with RDS MySQL:

```bash
export DATABASE_TYPE=mysql
export MYSQL_HOST=your-rds-endpoint.region.rds.amazonaws.com
export MYSQL_PORT=3306
export MYSQL_DATABASE=bmad_bot
export MYSQL_USERNAME=your-username
export MYSQL_PASSWORD=your-password
```

## Data Migration

### Migrating from SQLite to MySQL

The bot includes built-in migration support:

```go
// Example migration code (can be adapted for CLI tool)
sqliteService := storage.NewSQLiteStorageService("./data/bot_state.db")
mysqlService := storage.NewMySQLStorageService(mysqlConfig)

migrationService := storage.NewMigrationService(sqliteService, mysqlService)
err := migrationService.MigrateData(context.Background())
if err != nil {
    log.Fatal("Migration failed:", err)
}

// Validate migration
err = migrationService.ValidateMigration(context.Background())
if err != nil {
    log.Fatal("Migration validation failed:", err)
}
```

## Monitoring and Health Checks

### Health Check Endpoint

The bot includes built-in health checks that work with both SQLite and MySQL:

```bash
# Docker health check
docker exec bmad-discord-bot /app/main --health-check

# Kubernetes liveness probe
livenessProbe:
  exec:
    command:
    - /app/main
    - --health-check
  initialDelaySeconds: 30
  periodSeconds: 30
```

### MySQL Connection Monitoring

The MySQL implementation includes:
- Connection retry with exponential backoff
- Connection pooling optimization
- Comprehensive error logging
- Graceful degradation on connection failures

Monitor MySQL performance using:
```sql
-- Connection status
SHOW STATUS LIKE 'Threads_connected';

-- Query performance
SHOW STATUS LIKE 'Slow_queries';

-- Table usage
SELECT * FROM information_schema.tables 
WHERE table_schema = 'bmad_bot';
```

## Performance Considerations

### Connection Pooling

MySQL connections are pooled with the following defaults:
- MaxOpenConns: 10
- MaxIdleConns: 5
- ConnMaxLifetime: 1 hour

Adjust based on your deployment scale:

```go
// In production, consider increasing limits
db.SetMaxOpenConns(50)
db.SetMaxIdleConns(25)
db.SetConnMaxLifetime(time.Hour * 2)
```

### Index Optimization

The MySQL schema includes optimized indexes:
- `idx_message_states_channel_thread` for message lookups
- `idx_message_states_timestamp` for time-based queries
- `idx_thread_ownerships_thread_id` for thread ownership
- `idx_thread_ownerships_creation_time` for cleanup operations

## Security

### Database Security

1. **Use strong passwords**: Generate cryptographically secure passwords
2. **Network isolation**: Keep MySQL on private networks
3. **TLS encryption**: Enable TLS for database connections
4. **Limited privileges**: Create application-specific database users

### Example MySQL User Setup

```sql
-- Create application database
CREATE DATABASE bmad_bot CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Create application user with limited privileges
CREATE USER 'bmad_user'@'%' IDENTIFIED BY 'secure_password_here';
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, INDEX ON bmad_bot.* TO 'bmad_user'@'%';
FLUSH PRIVILEGES;
```

## Troubleshooting

See the [MySQL Troubleshooting Guide](troubleshooting-mysql.md) for common issues and solutions.

## Backup and Recovery

### Automated Backups

```bash
# MySQL dump backup
mysqldump -h $MYSQL_HOST -u $MYSQL_USERNAME -p$MYSQL_PASSWORD bmad_bot > backup_$(date +%Y%m%d_%H%M%S).sql

# Kubernetes CronJob for automated backups
apiVersion: batch/v1
kind: CronJob
metadata:
  name: mysql-backup
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: mysql-backup
            image: mysql:8.0
            command:
            - /bin/bash
            - -c
            - mysqldump -h mysql -u bmad_user -p$MYSQL_PASSWORD bmad_bot > /backup/backup_$(date +%Y%m%d_%H%M%S).sql
            env:
            - name: MYSQL_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: mysql-secret
                  key: user-password
            volumeMounts:
            - name: backup-storage
              mountPath: /backup
          volumes:
          - name: backup-storage
            persistentVolumeClaim:
              claimName: backup-pvc
          restartPolicy: OnFailure
```

### Point-in-Time Recovery

Enable MySQL binary logging for point-in-time recovery:

```sql
-- Enable binary logging
SET GLOBAL log_bin = ON;
SET GLOBAL binlog_format = 'ROW';
```

## Scaling Considerations

### Read Replicas

For high-read workloads, consider MySQL read replicas:

```go
// Example: Configure read/write split
writeDB := storage.NewMySQLStorageService(writeConfig)
readDB := storage.NewMySQLStorageService(readConfig)
```

### Horizontal Scaling

The bot's stateless design supports horizontal scaling with MySQL:
- Multiple bot instances can share the same MySQL database
- Use Kubernetes Horizontal Pod Autoscaler for automatic scaling
- Consider connection pooling limits when scaling

This completes the MySQL deployment guide. For additional support, refer to the troubleshooting documentation or project issues.