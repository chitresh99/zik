# Zik

A centralized configuration service written in Go. Instead of hardcoding values like `DB_URL`, `API_KEY`, or `MAX_RETRIES` inside every application, services fetch their configuration from this service at runtime over HTTP.

Every config value is versioned. Updating a key archives the previous value instead of overwriting it, so any key can be rolled back to a prior version at any time.

## Why this exists

This project was built to learn Go and backend infrastructure patterns by building something. It covers HTTP API design, concurrency safety, dependency injection through interfaces, SQL schema design for versioned data, and transactional writes with Postgres.

## Core concepts

**Namespace**
A logical grouping of configs, usually one per service or environment. Examples: `prod-env`, `dev-env`.

**Key**
A single configuration name within a namespace, such as `db_url` or `max_retries`.

**Version**
Every time a key's value is updated, the previous value is preserved as a version in its history. Versions are never deleted unless the key itself is deleted, except when rolled back to.

## Example usage

```
# Set a config value
POST /namespaces/prod-env/configs/db_url
{ "value": "postgres://prod-host:5432/cerebellum" }

# Get the current value
GET /namespaces/prod-env/configs/db_url

# Update it (old value becomes a version in history)
POST /namespaces/prod-env/configs/db_url
{ "value": "postgres://new-host:5432/production" }

# List every key in a namespace
GET /namespaces/prod-env/configs

# Roll back to the previous version
POST /namespaces/prod-env/configs/db_url/rollback

# Delete a key entirely
DELETE /namespaces/prod-env/configs/db_url
```

## Architecture

The project is split into three layers with a clear separation of concerns.

```
main.go              entrypoint, wires dependencies, starts the HTTP server
internal/store/       storage layer, knows nothing about HTTP
internal/handler/     HTTP layer, knows nothing about storage internals
```

### Store interface

The handler layer depends on an interface, not a concrete storage implementation.

```go
type Store interface {
    Set(namespace, key, value string) (*ConfigVersion, error)
    Get(namespace, key string) (*ConfigEntry, error)
    Delete(namespace, key string) error
    List(namespace string) (map[string]ConfigVersion, error)
    Rollback(namespace, key string) (*ConfigVersion, error)
}
```

Two implementations satisfy this interface:

**MemoryStore** (`internal/store/memory.go`)
An in-memory store using a nested map protected by a `sync.RWMutex`. Used as a fallback when no database is configured, and used in the test suite so tests run fast without needing a live database connection.

**PostgresStore** (`internal/store/postgres.go`)
A Postgres-backed store using `pgx`, designed for use with Neon. Writes are transactional, so an update to a config and the archiving of its previous version either both succeed or both fail.

`main.go` decides which implementation to use at startup based on whether `DATABASE_URL` is set, and passes it into the handler as the interface type. The handler code itself never changes regardless of which store is active.

## Database schema

```sql
CREATE TABLE configs (
    id         SERIAL PRIMARY KEY,
    namespace  TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    version    INT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(namespace, key)
);

CREATE TABLE config_versions (
    id         SERIAL PRIMARY KEY,
    namespace  TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    version    INT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
```

`configs` always holds the current value for each namespace and key pair. `config_versions` is an append only history table. On update, the current row is archived into `config_versions` before being overwritten. On rollback, the most recent row in `config_versions` is restored into `configs` and removed from history.

## Running locally

### In-memory mode (no setup required)

```bash
go run main.go
```

The service starts on port 8080 using the in-memory store.

### With Postgres (Neon)

1. Create a Neon project and copy the connection string.
2. Run the schema:

```bash
psql "$DATABASE_URL" -f migrations/001_create_configs_table.sql
psql "$DATABASE_URL" -f migrations/002_config_versions.sql
psql "$DATABASE_URL" -f migrations/003_create_index_config_versions.sql
```

3. Create a `.env` file in the project root:

```
DATABASE_URL=
PORT=
```

4. Run the service:

```bash
go mod tidy
go run main.go
```

The service will log that it connected to Postgres and use it for all storage.

## API reference

| Method | Path                                              | Description                          |
|--------|----------------------------------------------------|---------------------------------------|
| POST   | `/namespaces/{namespace}/configs/{key}`            | Create or update a key               |
| GET    | `/namespaces/{namespace}/configs/{key}`            | Get the current value and history    |
| DELETE | `/namespaces/{namespace}/configs/{key}`            | Delete a key and its history         |
| GET    | `/namespaces/{namespace}/configs`                  | List all keys in a namespace         |
| POST   | `/namespaces/{namespace}/configs/{key}/rollback`   | Roll back to the previous version    |
| GET    | `/health`                                          | Health check                         |