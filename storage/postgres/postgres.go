// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package postgres implements PostgreSQL as a hashR storage.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/hashr/core/hashr"

	// Blank import below is needed for the SQL driver.
	_ "github.com/lib/pq"
)

// Storage allows to interact with PostgreSQL instance.
type Storage struct {
	sqlDB *sql.DB
}

// NewStorage creates new Storage struct that allows to interact with PostgreSQL instance and all the necessary tables, if they don't exist.
func NewStorage(sqlDB *sql.DB) (*Storage, error) {
	// Check if the "jobs" table exists.
	exists, err := tableExists(sqlDB, "jobs")
	if err != nil {
		return nil, fmt.Errorf("error while checking if jobs table exists: %v", err)
	}

	if !exists {
		sql := `CREATE TABLE jobs (
		quick_sha256 VARCHAR(100) PRIMARY KEY,
		imported_at INT NOT NULL,
		id text,
		repo text,
		repo_path text,
		location text,
		sha256 VARCHAR(100),
		status VARCHAR(50),
		error text,
		preprocessing_duration INT,
		processing_duration INT,
		export_duration INT,
		files_extracted INT,
		files_exported INT
	  )`
		_, err = sqlDB.Exec(sql)
		if err != nil {
			return nil, fmt.Errorf("error while creating jobs table: %v", err)
		}
	}

	return &Storage{sqlDB: sqlDB}, nil
}

func (s *Storage) rowExists(qHash string) (bool, error) {
	sqlStatement := `SELECT quick_sha256 FROM jobs WHERE quick_sha256=$1;`
	var quickSha256 string
	row := s.sqlDB.QueryRow(sqlStatement, qHash)
	switch err := row.Scan(&quickSha256); err {
	case sql.ErrNoRows:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, err
	}
}

// UpdateJobs updates cloud spanner table.
func (s *Storage) UpdateJobs(ctx context.Context, qHash string, p *hashr.ProcessingSource) error {
	exists, err := s.rowExists(qHash)
	if err != nil {
		return err
	}

	var sql string
	if exists {
		sql = `
UPDATE jobs SET imported_at = $2, id = $3, repo = $4, repo_path = $5, location = $6, sha256 = $7, status = $8, error = $9, preprocessing_duration = $10, processing_duration = $11, export_duration = $12, files_extracted = $13, files_exported = $14
WHERE quick_sha256 = $1`
	} else {
		sql = `
INSERT INTO jobs (quick_sha256,  imported_at, id, repo, repo_path, location, sha256, status, error, preprocessing_duration, processing_duration, export_duration, files_extracted, files_exported)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`
	}

	_, err = s.sqlDB.Exec(sql, qHash, p.ImportedAt, p.ID, p.Repo, p.RepoPath, p.RemoteSourcePath, p.Sha256, p.Status, p.Error, int(p.PreprocessingDuration.Seconds()), int(p.ProcessingDuration.Seconds()), int(p.ExportDuration.Seconds()), p.SampleCount, p.ExportCount)
	if err != nil {
		return err
	}
	return nil
}

// FetchJobs fetches processing jobs from cloud spanner.
func (s *Storage) FetchJobs(ctx context.Context) (map[string]string, error) {
	processed := make(map[string]string)

	rows, err := s.sqlDB.Query("SELECT quick_sha256, status FROM jobs")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var quickSha256, status string
		err = rows.Scan(&quickSha256, &status)
		if err != nil {
			return nil, err
		}
		processed[quickSha256] = status
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return processed, nil
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
