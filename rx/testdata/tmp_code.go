package rx

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// Migration съдържа метаданни за миграция
type Migration struct {
	Version   string
	Direction string // "up" или "down"
	SQL       string
}

// parseMigrationFile парсва файл и връща Migration
func parseMigrationFile(filePath string) (*Migration, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open file %q: %w", filePath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// Четем първия ред
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty migration file %q", filePath)
	}
	header := strings.TrimSpace(scanner.Text())

	if !strings.HasPrefix(header, "--") {
		return nil, fmt.Errorf("invalid migration header in %q, expected comment starting with --", filePath)
	}

	parts := strings.Fields(strings.TrimPrefix(header, "--"))
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid migration header format in %q: %s", filePath, header)
	}

	version, direction := parts[0], strings.ToLower(parts[1])
	if direction != "up" && direction != "down" {
		return nil, fmt.Errorf("invalid migration direction %q in %q", direction, filePath)
	}

	// Останалото е SQL
	var sqlBuilder strings.Builder
	for scanner.Scan() {
		sqlBuilder.WriteString(scanner.Text())
		sqlBuilder.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", filePath, err)
	}

	return &Migration{
		Version:   version,
		Direction: direction,
		SQL:       sqlBuilder.String(),
	}, nil
}

// ensureSchemaMigrationsTable създава таблица schema_migrations, ако не съществува
func ensureSchemaMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT NOT NULL,
			direction TEXT NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(version, direction)
		)
	`)
	return err
}

// migrationExists проверява дали дадена миграция вече е била приложена
func migrationExists(ctx context.Context, db *sql.DB, version, direction string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM schema_migrations WHERE version = ? AND direction = ?`,
		version, direction,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Migrate зарежда SQL команди от файл и ги изпълнява в SQLite база данни.
// Ако миграцията вече е прилагана, тя се пропуска.
func Migrate(ctx context.Context, dbPath, sqlFilePath string) (*Migration, error) {
	migration, err := parseMigrationFile(sqlFilePath)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open database %q: %w", dbPath, err)
	}
	defer db.Close()

	// Уверяваме се, че таблицата съществува
	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		return nil, fmt.Errorf("cannot ensure schema_migrations table: %w", err)
	}

	// Проверяваме дали вече е прилагана
	exists, err := migrationExists(ctx, db, migration.Version, migration.Direction)
	if err != nil {
		return nil, fmt.Errorf("cannot check existing migrations: %w", err)
	}
	if exists {
		return migration, nil // вече е прилагана, връщаме без грешка
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot begin transaction: %w", err)
	}

	_, err = tx.ExecContext(ctx, migration.SQL)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("migration %s %s failed: %w", migration.Version, migration.Direction, err)
	}

	// Записваме в schema_migrations
	_, err = tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, direction) VALUES (?, ?)`,
		migration.Version, migration.Direction,
	)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("cannot record migration %s %s: %w", migration.Version, migration.Direction, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cannot commit migration %s %s: %w", migration.Version, migration.Direction, err)
	}

	return migration, nil
}
