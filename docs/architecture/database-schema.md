# Database Schema

## SQLite Database: Message State Persistence

The bot uses a local SQLite database to persist message tracking state across restarts.

### Database File Location
- Default: `./data/bot_state.db`
- Configurable via `DATABASE_PATH` environment variable
- Directory is auto-created if it doesn't exist

### Table: message_states

Stores the last seen message information per Discord channel/thread for recovery after bot restarts.

```sql
CREATE TABLE message_states (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id TEXT NOT NULL,
    thread_id TEXT NULL,
    last_message_id TEXT NOT NULL,
    last_seen_timestamp INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE(channel_id, thread_id)
);

-- Index for efficient lookups during startup recovery
CREATE INDEX idx_message_states_channel_thread ON message_states(channel_id, thread_id);
CREATE INDEX idx_message_states_timestamp ON message_states(last_seen_timestamp);
```

### Schema Details

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | INTEGER | PRIMARY KEY AUTOINCREMENT | Unique identifier for each record |
| `channel_id` | TEXT | NOT NULL | Discord channel ID where the message was processed |
| `thread_id` | TEXT | NULL | Discord thread ID (null for regular channels) |
| `last_message_id` | TEXT | NOT NULL | ID of the last processed message |
| `last_seen_timestamp` | INTEGER | NOT NULL | Unix timestamp of the last processed message |
| `created_at` | INTEGER | NOT NULL | Unix timestamp when record was created |
| `updated_at` | INTEGER | NOT NULL | Unix timestamp when record was last updated |

### Constraints and Indexes

- **UNIQUE(channel_id, thread_id)**: Ensures only one record per channel/thread combination
- **idx_message_states_channel_thread**: Optimizes lookups during message processing
- **idx_message_states_timestamp**: Optimizes time-based queries during startup recovery

### Recovery Logic

The bot uses this schema to implement message recovery after restarts:
1. Query all records to get last seen timestamps per channel/thread
2. Compare against configurable recovery window (default: 5 minutes)
3. Fetch missed messages from Discord API within the time window
4. Process messages in chronological order
5. Update records with new last seen information