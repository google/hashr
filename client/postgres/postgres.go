// Package postgres implements PostgreSQL as a hashR storage.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	// Blank import below is needed for the SQL driver.
	_ "github.com/lib/pq"
)

// Storage allows to interact with PostgreSQL instance.
type Storage struct {
	sqlDB *sql.DB
}

// NewStorage creates new Storage struct that allows to interact with PostgreSQL instance and all the necessary tables, if they don't exist.
func NewStorage(sqlDB *sql.DB) (*Storage, error) {
	return &Storage{sqlDB: sqlDB}, nil
}

// GetSamples fetches processed samples from postgres.
func (s *Storage) GetSamples(ctx context.Context) (map[string]map[string]string, error) {
	exists, err := tableExists(s.sqlDB, "samples")
	if err != nil {
		return nil, err
	}

	samples := make(map[string]map[string]string)

	if exists {
		var sql = `SELECT * FROM samples;`

		rows, err := s.sqlDB.Query(sql)

		if err != nil {
			return nil, err
		}

		defer rows.Close()

		for rows.Next() {
			var sha256, mimetype, fileOutput, size string
			err := rows.Scan(&sha256, &mimetype, &fileOutput, &size)
			if err != nil {
				return nil, err
			}

			samples[sha256] = make(map[string]string)

			// Assign values to the nested map
			samples[sha256]["sha256"] = sha256
			samples[sha256]["mimetype"] = mimetype
			samples[sha256]["file_output"] = fileOutput
			samples[sha256]["size"] = size
		}

	} else {
		return nil, fmt.Errorf("table samples does not exist")
	}

	return samples, nil
}

func tableExists(db *sql.DB, tableName string) (bool, error) {
	// Query to check if the table exists in PostgreSQL
	query := `
        SELECT EXISTS (
            SELECT 1
            FROM   information_schema.tables
            WHERE  table_name = $1
        )
    `

	var exists bool
	err := db.QueryRow(query, tableName).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}
