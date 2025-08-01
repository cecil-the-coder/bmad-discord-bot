# MySQL Troubleshooting Guide

This guide covers common issues and solutions when using MySQL with the BMAD Discord Bot.

## Connection Issues

### Error: "failed to connect after X attempts"

**Symptoms:**
```
Failed to initialize storage service: failed to establish database connection: failed to connect after 5 attempts
```

**Causes & Solutions:**

1. **MySQL server not running**
   ```bash
   # Check MySQL status
   systemctl status mysql
   # Or for Docker
   docker ps | grep mysql
   
   # Start MySQL
   systemctl start mysql
   # Or for Docker
   docker-compose up mysql
   ```

2. **Incorrect connection parameters**
   ```bash
   # Verify environment variables
   echo $MYSQL_HOST
   echo $MYSQL_PORT
   echo $MYSQL_DATABASE
   echo $MYSQL_USERNAME
   # Don't echo password for security
   
   # Test connection manually
   mysql -h $MYSQL_HOST -P $MYSQL_PORT -u $MYSQL_USERNAME -p$MYSQL_PASSWORD -e "SELECT 1"
   ```

3. **Network connectivity issues**
   ```bash
   # Test network connectivity
   telnet $MYSQL_HOST $MYSQL_PORT
   # Or
   nc -zv $MYSQL_HOST $MYSQL_PORT
   
   # Check firewall rules
   sudo ufw status
   # Or for Docker networks
   docker network ls
   docker network inspect bmad-discord-bot_default
   ```

4. **MySQL bind address configuration**
   ```sql
   -- Check MySQL bind address
   SHOW VARIABLES LIKE 'bind_address';
   
   -- Should be '0.0.0.0' or '*' for external connections
   ```

### Error: "Access denied for user"

**Symptoms:**
```
attempt 1: failed to ping database: Error 1045: Access denied for user 'bmad_user'@'172.18.0.3' (using password: YES)
```

**Solutions:**

1. **Verify user credentials**
   ```sql
   -- Check if user exists
   SELECT User, Host FROM mysql.user WHERE User = 'bmad_user';
   
   -- Reset password if needed
   ALTER USER 'bmad_user'@'%' IDENTIFIED BY 'new_secure_password';
   FLUSH PRIVILEGES;
   ```

2. **Check user permissions**
   ```sql
   -- Show user grants
   SHOW GRANTS FOR 'bmad_user'@'%';
   
   -- Grant necessary permissions
   GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, INDEX ON bmad_bot.* TO 'bmad_user'@'%';
   FLUSH PRIVILEGES;
   ```

3. **Host/IP restrictions**
   ```sql
   -- Create user for specific host pattern
   CREATE USER 'bmad_user'@'172.18.%' IDENTIFIED BY 'secure_password';
   GRANT ALL PRIVILEGES ON bmad_bot.* TO 'bmad_user'@'172.18.%';
   
   -- Or allow from any host (less secure)
   CREATE USER 'bmad_user'@'%' IDENTIFIED BY 'secure_password';
   ```

### Error: "Unknown database"

**Symptoms:**
```
failed to ping database: Error 1049: Unknown database 'bmad_bot'
```

**Solutions:**

1. **Create the database**
   ```sql
   CREATE DATABASE bmad_bot CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
   ```

2. **Verify database name in environment**
   ```bash
   # Check environment variable
   echo $MYSQL_DATABASE
   
   # List all databases
   mysql -h $MYSQL_HOST -u $MYSQL_USERNAME -p -e "SHOW DATABASES;"
   ```

## Schema and Migration Issues

### Error: "Table doesn't exist"

**Symptoms:**
```
database health check query failed: Error 1146: Table 'bmad_bot.message_states' doesn't exist
```

**Solutions:**

1. **Check table creation**
   ```sql
   -- List tables in database
   USE bmad_bot;
   SHOW TABLES;
   
   -- Check table structure
   DESCRIBE message_states;
   DESCRIBE thread_ownerships;
   ```

2. **Manual table creation** (if auto-creation failed)
   ```sql
   USE bmad_bot;
   
   CREATE TABLE IF NOT EXISTS message_states (
       id BIGINT PRIMARY KEY AUTO_INCREMENT,
       channel_id VARCHAR(255) NOT NULL,
       thread_id VARCHAR(255) NULL,
       last_message_id VARCHAR(255) NOT NULL,
       last_seen_timestamp BIGINT NOT NULL,
       created_at BIGINT NOT NULL,
       updated_at BIGINT NOT NULL,
       UNIQUE KEY unique_channel_thread (channel_id, thread_id)
   );
   
   CREATE TABLE IF NOT EXISTS thread_ownerships (
       id BIGINT PRIMARY KEY AUTO_INCREMENT,
       thread_id VARCHAR(255) NOT NULL UNIQUE,
       original_user_id VARCHAR(255) NOT NULL,
       created_by VARCHAR(255) NOT NULL,
       creation_time BIGINT NOT NULL,
       created_at BIGINT NOT NULL,
       updated_at BIGINT NOT NULL
   );
   
   -- Create indexes
   CREATE INDEX IF NOT EXISTS idx_message_states_channel_thread ON message_states(channel_id, thread_id);
   CREATE INDEX IF NOT EXISTS idx_message_states_timestamp ON message_states(last_seen_timestamp);
   CREATE INDEX IF NOT EXISTS idx_thread_ownerships_thread_id ON thread_ownerships(thread_id);
   CREATE INDEX IF NOT EXISTS idx_thread_ownerships_creation_time ON thread_ownerships(creation_time);
   ```

### Migration Issues

**Error: Data migration fails**

1. **Check source SQLite database**
   ```bash
   # Verify SQLite database exists and is readable
   ls -la ./data/bot_state.db
   sqlite3 ./data/bot_state.db ".tables"
   sqlite3 ./data/bot_state.db "SELECT COUNT(*) FROM message_states;"
   ```

2. **Verify destination MySQL setup**
   ```sql
   -- Check MySQL connection and permissions
   USE bmad_bot;
   SELECT COUNT(*) FROM message_states;
   SELECT COUNT(*) FROM thread_ownerships;
   ```

3. **Run migration with verbose logging**
   ```go
   // Enable detailed logging during migration
   migrationService := storage.NewMigrationService(sqliteService, mysqlService)
   
   // Add logging to track progress
   err := migrationService.MigrateData(ctx)
   if err != nil {
       log.Printf("Migration failed: %v", err)
   }
   
   // Validate results
   err = migrationService.ValidateMigration(ctx)
   if err != nil {
       log.Printf("Migration validation failed: %v", err)
   }
   ```

## Performance Issues

### Slow Query Performance

**Symptoms:**
- Bot responds slowly to Discord messages
- High CPU usage on MySQL server
- Connection timeouts

**Diagnosis:**

1. **Enable MySQL slow query log**
   ```sql
   SET GLOBAL slow_query_log = 'ON';
   SET GLOBAL long_query_time = 1;  -- Log queries taking > 1 second
   SET GLOBAL slow_query_log_file = '/var/log/mysql/slow.log';
   ```

2. **Check running processes**
   ```sql
   SHOW PROCESSLIST;
   SHOW STATUS LIKE 'Threads_connected';
   SHOW STATUS LIKE 'Slow_queries';
   ```

3. **Analyze query execution**
   ```sql
   -- Check index usage
   EXPLAIN SELECT * FROM message_states WHERE channel_id = 'test' AND thread_id IS NULL;
   
   -- Show index statistics
   SHOW INDEX FROM message_states;
   SHOW INDEX FROM thread_ownerships;
   ```

**Solutions:**

1. **Optimize indexes**
   ```sql
   -- Analyze table for index recommendations
   ANALYZE TABLE message_states;
   ANALYZE TABLE thread_ownerships;
   
   -- Add missing indexes if needed
   CREATE INDEX idx_message_states_last_seen ON message_states(last_seen_timestamp);
   ```

2. **Adjust connection pool settings**
   ```go
   // Increase connection limits for high-load environments
   db.SetMaxOpenConns(50)
   db.SetMaxIdleConns(25)
   db.SetConnMaxLifetime(time.Hour * 2)
   ```

3. **MySQL configuration optimization**
   ```ini
   # /etc/mysql/mysql.conf.d/mysqld.cnf
   [mysqld]
   innodb_buffer_pool_size = 1G
   innodb_log_file_size = 256M
   max_connections = 200
   query_cache_size = 32M
   query_cache_type = 1
   ```

### Memory Issues

**Symptoms:**
```
MySQL server has gone away
Lost connection to MySQL server during query
```

**Solutions:**

1. **Increase MySQL timeouts**
   ```sql
   SET GLOBAL wait_timeout = 600;
   SET GLOBAL interactive_timeout = 600;
   SET GLOBAL max_allowed_packet = 64M;
   ```

2. **Monitor memory usage**
   ```bash
   # Check MySQL memory usage
   mysql -e "SHOW STATUS LIKE 'innodb_buffer_pool_pages_data';"
   mysql -e "SHOW VARIABLES LIKE 'innodb_buffer_pool_size';"
   
   # System memory
   free -h
   ```

3. **Adjust buffer pool size**
   ```sql
   -- Check current buffer pool usage
   SELECT 
       (SELECT COUNT(*) FROM information_schema.innodb_buffer_page) as total_pages,
       (SELECT COUNT(*) FROM information_schema.innodb_buffer_page WHERE page_type='FILE_PAGE') as data_pages;
   ```

## Docker-Specific Issues

### Container Communication Issues

**Error: "connection refused" between containers**

1. **Check Docker network**
   ```bash
   # List Docker networks
   docker network ls
   
   # Inspect network configuration
   docker network inspect bmad-discord-bot_default
   
   # Check container connectivity
   docker exec bmad-discord-bot ping mysql
   ```

2. **Verify service names**
   ```yaml
   # In docker-compose.yml, use service names for host
   environment:
     MYSQL_HOST: mysql  # Not 'localhost'
     MYSQL_PORT: 3306
   ```

3. **Check port exposure**
   ```yaml
   # MySQL service should expose port internally
   mysql:
     ports:
       - "3306"  # Internal port
       # - "3306:3306"  # Uncomment only if external access needed
   ```

### Volume Permission Issues

**Error: MySQL data directory permissions**

1. **Fix volume permissions**
   ```bash
   # Check volume ownership
   docker-compose exec mysql ls -la /var/lib/mysql
   
   # Fix permissions if needed
   sudo chown -R 999:999 ./mysql-data
   ```

2. **SELinux issues (if applicable)**
   ```bash
   # Check SELinux context
   ls -Z ./mysql-data
   
   # Fix SELinux context
   sudo setsebool -P container_manage_cgroup on
   sudo chcon -Rt svirt_sandbox_file_t ./mysql-data
   ```

## Kubernetes-Specific Issues

### Pod Communication Issues

1. **Check service discovery**
   ```bash
   # From bot pod, test MySQL service
   kubectl exec -it bmad-discord-bot-pod -- nslookup mysql
   kubectl exec -it bmad-discord-bot-pod -- ping mysql
   ```

2. **Verify service configuration**
   ```bash
   kubectl get svc mysql
   kubectl describe svc mysql
   kubectl get endpoints mysql
   ```

### Persistent Volume Issues

1. **Check PVC status**
   ```bash
   kubectl get pvc
   kubectl describe pvc mysql-pvc
   ```

2. **Storage class issues**
   ```bash
   kubectl get storageclass
   kubectl describe storageclass
   ```

## Logging and Monitoring

### Enable Detailed Logging

1. **Application logging**
   ```bash
   # Set log level for more details
   export LOG_LEVEL=debug
   
   # View container logs
   docker-compose logs -f bmad-bot
   docker-compose logs -f mysql
   ```

2. **MySQL query logging**
   ```sql
   -- Enable general query log
   SET GLOBAL general_log = 'ON';
   SET GLOBAL general_log_file = '/var/log/mysql/general.log';
   
   -- Monitor logs
   tail -f /var/log/mysql/general.log
   ```

### Health Check Debugging

1. **Manual health check**
   ```bash
   # Test bot health check
   docker exec bmad-discord-bot /app/main --health-check
   
   # Check MySQL health
   docker exec bmad-mysql mysqladmin ping -h localhost -u root -p
   ```

2. **Connection testing**
   ```bash
   # Test from bot container
   docker exec -it bmad-discord-bot mysql -h mysql -u bmad_user -p
   ```

## Getting Help

If you continue to experience issues:

1. **Collect diagnostic information**
   ```bash
   # System information
   docker --version
   docker-compose --version
   mysql --version
   
   # Container status
   docker-compose ps
   docker-compose logs --tail=50 bmad-bot
   docker-compose logs --tail=50 mysql
   
   # Environment variables (sanitized)
   env | grep -E "(DATABASE|MYSQL)" | sed 's/PASSWORD=.*/PASSWORD=***/'
   ```

2. **Check project issues**: Visit the GitHub repository issues page

3. **MySQL documentation**: Refer to official MySQL documentation for database-specific issues

4. **Docker documentation**: Check Docker and docker-compose documentation for container issues

## Prevention

### Regular Maintenance

1. **Monitor disk space**
   ```bash
   # Check MySQL data directory size
   du -sh /var/lib/mysql
   
   # Monitor log file sizes
   ls -lh /var/log/mysql/
   ```

2. **Backup verification**
   ```bash
   # Test backup restoration
   mysqldump -h $MYSQL_HOST -u $MYSQL_USERNAME -p$MYSQL_PASSWORD bmad_bot > test_backup.sql
   mysql -h $MYSQL_HOST -u $MYSQL_USERNAME -p$MYSQL_PASSWORD -e "CREATE DATABASE test_restore;"
   mysql -h $MYSQL_HOST -u $MYSQL_USERNAME -p$MYSQL_PASSWORD test_restore < test_backup.sql
   ```

3. **Performance monitoring**
   ```bash
   # Set up monitoring with tools like:
   # - Prometheus + Grafana
   # - MySQL Enterprise Monitor
   # - Percona Monitoring and Management (PMM)
   ```

This troubleshooting guide covers the most common issues. For complex problems, consider consulting MySQL documentation or seeking community support.