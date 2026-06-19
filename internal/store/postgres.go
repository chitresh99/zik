package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) Set(namespace, key, value string) (*ConfigVersion, error) {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentValue string
	var currentVersion int
	var currentUpdatedAt time.Time

	err = tx.QueryRow(ctx,
		`SELECT value, version, updated_at FROM configs WHERE namespace=$1 AND key=$2`,
		namespace, key,
	).Scan(&currentValue, &currentVersion, &currentUpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		now := time.Now().UTC()
		_, err = tx.Exec(ctx,
			`INSERT INTO configs (namespace, key, value, version, updated_at)
             VALUES ($1, $2, $3, 1, $4)`,
			namespace, key, value, now,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert config: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
		return &ConfigVersion{Value: value, Version: 1, UpdatedAt: now}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query existing config: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO config_versions (namespace, key, value, version, updated_at)
         VALUES ($1, $2, $3, $4, $5)`,
		namespace, key, currentValue, currentVersion, currentUpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to archive previous version: %w", err)
	}

	newVersion := currentVersion + 1
	now := time.Now().UTC()
	_, err = tx.Exec(ctx,
		`UPDATE configs SET value=$1, version=$2, updated_at=$3
         WHERE namespace=$4 AND key=$5`,
		value, newVersion, now, namespace, key,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update config: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &ConfigVersion{Value: value, Version: newVersion, UpdatedAt: now}, nil
}

func (s *PostgresStore) Get(namespace, key string) (*ConfigEntry, error) {
	ctx := context.Background()

	var current ConfigVersion
	err := s.pool.QueryRow(ctx,
		`SELECT value, version, updated_at FROM configs WHERE namespace=$1 AND key=$2`,
		namespace, key,
	).Scan(&current.Value, &current.Version, &current.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("key %q not found in namespace %q", key, namespace)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query config: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT value, version, updated_at FROM config_versions
         WHERE namespace=$1 AND key=$2 ORDER BY version ASC`,
		namespace, key,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	history := []ConfigVersion{}
	for rows.Next() {
		var v ConfigVersion
		if err := rows.Scan(&v.Value, &v.Version, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan history row: %w", err)
		}
		history = append(history, v)
	}

	return &ConfigEntry{Current: current, History: history}, nil
}

func (s *PostgresStore) Delete(namespace, key string) error {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`DELETE FROM configs WHERE namespace=$1 AND key=$2`,
		namespace, key,
	)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("key %q not found in namespace %q", key, namespace)
	}

	_, err = tx.Exec(ctx,
		`DELETE FROM config_versions WHERE namespace=$1 AND key=$2`,
		namespace, key,
	)
	if err != nil {
		return fmt.Errorf("failed to delete config history: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *PostgresStore) List(namespace string) (map[string]ConfigVersion, error) {
	ctx := context.Background()

	rows, err := s.pool.Query(ctx,
		`SELECT key, value, version, updated_at FROM configs WHERE namespace=$1`,
		namespace,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query namespace: %w", err)
	}
	defer rows.Close()

	result := make(map[string]ConfigVersion)
	for rows.Next() {
		var key string
		var v ConfigVersion
		if err := rows.Scan(&key, &v.Value, &v.Version, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		result[key] = v
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("namespace %q not found or empty", namespace)
	}

	return result, nil
}

func (s *PostgresStore) Rollback(namespace, key string) (*ConfigVersion, error) {
	ctx := context.Background()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var prev ConfigVersion
	var versionRowID int
	err = tx.QueryRow(ctx,
		`SELECT id, value, version, updated_at FROM config_versions
         WHERE namespace=$1 AND key=$2
         ORDER BY version DESC LIMIT 1`,
		namespace, key,
	).Scan(&versionRowID, &prev.Value, &prev.Version, &prev.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no previous version to roll back to for key %q", key)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query history: %w", err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE configs SET value=$1, version=$2, updated_at=$3
         WHERE namespace=$4 AND key=$5`,
		prev.Value, prev.Version, prev.UpdatedAt, namespace, key,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to restore previous version: %w", err)
	}

	_, err = tx.Exec(ctx, `DELETE FROM config_versions WHERE id=$1`, versionRowID)
	if err != nil {
		return nil, fmt.Errorf("failed to remove rolled-back version from history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &prev, nil
}
