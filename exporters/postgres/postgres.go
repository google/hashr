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

// Package postgres provides functions required to export data to PostgreSQL.
package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/golang/glog"

	"github.com/google/hashr/common"

	"github.com/lib/pq"
)

const (
	// Name contains name of the exporter.
	Name = "postgres"
)

// Exporter is an instance of Postgres Exporter.
type Exporter struct {
	sqlDB          *sql.DB
	uploadPayloads bool
}

// Name returns exporter name.
func (e *Exporter) Name() string {
	return Name
}

// NewExporter creates new Postregre exporter and all the necessary tables, if they don't exist.
func NewExporter(sqlDB *sql.DB, uploadPayloads bool) (*Exporter, error) {
	// Check if the "samples" table exists.
	exists, err := tableExists(sqlDB, "samples")
	if err != nil {
		return nil, fmt.Errorf("error while checking if samples table exists: %v", err)
	}

	if !exists {
		sql := `CREATE TABLE samples (
				sha256 VARCHAR(100)  PRIMARY KEY,
				mimetype text,
				file_output  text,
				size INT
		  )`
		_, err = sqlDB.Exec(sql)
		if err != nil {
			return nil, fmt.Errorf("error while creating samples table: %v", err)
		}
	}

	// Check if the "payloads" table exists.
	exists, err = tableExists(sqlDB, "payloads")
	if err != nil {
		return nil, fmt.Errorf("error while checking if payloads table exists: %v", err)
	}

	if !exists {
		sql := `CREATE TABLE payloads (
				sha256 VARCHAR(100)  PRIMARY KEY,
				payload bytea
		  )`
		_, err = sqlDB.Exec(sql)
		if err != nil {
			return nil, fmt.Errorf("error while creating payloads table: %v", err)
		}
	}

	// Check if the "sources" table exists.
	exists, err = tableExists(sqlDB, "sources")
	if err != nil {
		return nil, fmt.Errorf("error while checking if sources table exists: %v", err)
	}

	if !exists {
		sql := `CREATE TABLE sources (
			sha256 VARCHAR(100)  PRIMARY KEY,
			sourceID  text[],
			sourcePath  text,
			sourceDescription text,
			repoName text,
			repoPath text
		  )`
		_, err = sqlDB.Exec(sql)
		if err != nil {
			return nil, fmt.Errorf("error while creating sources table: %v", err)
		}
	}

	// Check if the "samples_sources" table exists.
	exists, err = tableExists(sqlDB, "samples_sources")
	if err != nil {
		return nil, fmt.Errorf("error while checking if samples_sources table exists: %v", err)
	}

	if !exists {
		sql := `CREATE TABLE samples_sources (
			sample_sha256 VARCHAR(100) REFERENCES samples(sha256) NOT NULL,
			source_sha256 VARCHAR(100) REFERENCES sources(sha256) NOT NULL,
			sample_paths text[],
			PRIMARY KEY (sample_sha256, source_sha256)
		  )`
		_, err = sqlDB.Exec(sql)
		if err != nil {
			return nil, fmt.Errorf("error while creating samples_sources table: %v", err)
		}
	}

	return &Exporter{sqlDB: sqlDB, uploadPayloads: uploadPayloads}, nil
}

// Export exports extracted data to PostgreSQL instance.
func (e *Exporter) Export(ctx context.Context, sourceRepoName, sourceRepoPath, sourceID, sourceHash, sourcePath, sourceDescription string, samples []common.Sample) error {
	if err := e.insertSource(sourceHash, sourceID, sourcePath, sourceRepoName, sourceRepoPath, sourceDescription); err != nil {
		return fmt.Errorf("could not upload source data: %v", err)
	}

	for _, sample := range samples {
		exists, err := e.sampleExists(sample.Sha256)
		if err != nil {
			glog.Errorf("skipping %s, could not check if sample was already uploaded: %v", sample.Sha256, err)
			continue
		}

		if !exists {
			if err := e.insertSample(sample, e.uploadPayloads); err != nil {
				glog.Errorf("skipping %s, could not insert sample data: %v", sample.Sha256, err)
				continue
			}
		}

		if err := e.insertRelationship(sample, sourceHash); err != nil {
			glog.Errorf("skipping %s, could not insert source <-> sample relationship: %v", sample.Sha256, err)
			continue
		}
	}

	return nil
}

func (e *Exporter) sampleExists(sha256 string) (bool, error) {
	sqlStatement := `SELECT sha256 FROM samples WHERE sha256=$1;`
	var quickSha256 string
	row := e.sqlDB.QueryRow(sqlStatement, sha256)
	switch err := row.Scan(&quickSha256); err {
	case sql.ErrNoRows:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, err
	}
}

func (e *Exporter) relationshipExists(sourceSha256, sampleSha256 string) (bool, error) {
	sqlStatement := `
	SELECT sample_sha256,source_sha256 
	FROM samples_sources 
	WHERE sample_sha256=$1 AND source_sha256=$2;`
	row := e.sqlDB.QueryRow(sqlStatement, sampleSha256, sourceSha256)
	switch err := row.Scan(&sampleSha256, &sourceSha256); err {
	case sql.ErrNoRows:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, err
	}
}

func (e *Exporter) sourceExists(sha256 string) (bool, error) {
	sqlStatement := `
	SELECT sha256 
	FROM sources 
	WHERE sha256=$1;`
	var quickSha256 string
	row := e.sqlDB.QueryRow(sqlStatement, sha256)
	switch err := row.Scan(&quickSha256); err {
	case sql.ErrNoRows:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, err
	}
}

func (e *Exporter) insertSample(sample common.Sample, uploadPayload bool) error {
	sqlSamples := `
	INSERT INTO samples (sha256, size, mimetype, file_output)
	VALUES ($1, $2, $3, $4)`

	var samplePath string
	var fi os.FileInfo
	var err error
	// If sample has more than one path associated with it, take the first that is valid.
	for _, path := range sample.Paths {
		if fi, err = os.Stat(path); err == nil {
			samplePath = path
			break
		}
	}

	file, err := os.Open(samplePath)
	if err != nil {
		return fmt.Errorf("could not open %v", samplePath)
	}
	defer file.Close()

	mimeType, err := getFileContentType(file)
	if err != nil {
		glog.Warningf("Could not get file content type: %v", err)
	}

	fileOutput, err := fileCmdOutput(samplePath)
	if err != nil {
		glog.Warningf("Could not get file cmd output: %v", err)
	}

	fileOutput = strings.TrimPrefix(fileOutput, fmt.Sprintf("%s%s", samplePath, ":"))

	_, err = e.sqlDB.Exec(sqlSamples, sample.Sha256, int(fi.Size()), mimeType, fileOutput)
	if err != nil {
		return fmt.Errorf("could not execute SQL: %v", err)
	}

	if uploadPayload {
		data, err := os.ReadFile(samplePath)
		if err != nil {
			return fmt.Errorf("error while opening file: %v", err)
		}

		sqlPayloads := `
		INSERT INTO payloads (sha256, payload)
		VALUES ($1, $2)`

		_, err = e.sqlDB.Exec(sqlPayloads, sample.Sha256, data)
		if err != nil {
			return fmt.Errorf("could not execute SQL: %v", err)
		}
	}

	return nil
}

func (e *Exporter) insertSource(sourceHash, sourceID, sourcePath, sourceRepoName, sourceRepoPath, sourceDescription string) error {
	exists, err := e.sourceExists(sourceHash)
	if err != nil {
		return err
	}

	var sql string
	if !exists {
		sql = `
		INSERT INTO sources (sha256, sourceID, sourcePath, repoName, repoPath, sourceDescription)
		VALUES ($1, $2, $3, $4, $5, $6)`

		_, err = e.sqlDB.Exec(sql, sourceHash, pq.Array([]string{sourceID}), sourcePath, sourceRepoName, sourceRepoPath, sourceDescription)
		if err != nil {
			return err
		}
	} else {
		sql = `UPDATE sources set sourceID = array_append(sourceID, $1) WHERE sha256 = $2`
		_, err = e.sqlDB.Exec(sql, sourceID, sourceHash)
		if err != nil {
			return err
		}
	}

	return nil
}

func (e *Exporter) insertRelationship(sample common.Sample, sourceSha256 string) error {
	exists, err := e.relationshipExists(sourceSha256, sample.Sha256)
	if err != nil {
		return err
	}

	var paths []string

	for _, path := range sample.Paths {
		s := strings.Split(path, "/extracted/")
		if len(s) < 2 {
			glog.Warningf("sample path does not follow expected format: %s", path)
			continue
		}
		paths = append(paths, s[len(s)-1])
	}

	if !exists {
		sql := `
		INSERT INTO samples_sources (sample_sha256, source_sha256, sample_paths)
		VALUES ($1, $2, $3)`

		_, err = e.sqlDB.Exec(sql, sample.Sha256, sourceSha256, pq.Array(paths))
		if err != nil {
			return err
		}
	} else {
		sql := `
		UPDATE samples_sources set sample_paths =  array_cat(sample_paths, $1) 
		WHERE sample_sha256 = $2 AND source_sha256 = $3
		`
		_, err = e.sqlDB.Exec(sql, pq.Array(paths), sample.Sha256, sourceSha256)
		if err != nil {
			return err
		}
		// UPDATE users SET topics = array_cat(topics, '{cats,mice}');
	}

	return nil
}

func getFileContentType(out *os.File) (string, error) {

	// Only the first 512 bytes are used to check the content type.
	buffer := make([]byte, 512)

	_, err := out.Read(buffer)
	if err != nil {
		return "", err
	}

	contentType := http.DetectContentType(buffer)

	return contentType, nil
}

func fileCmdOutput(filepath string) (string, error) {
	cmd := exec.Command("/usr/bin/file", filepath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error while executing %s: %v\nStdout: %v\nStderr: %v", "/usr/bin/file", err, stdout.String(), stderr.String())
	}

	return strings.TrimSuffix(stdout.String(), "\n"), nil
}

func tableExists(db *sql.DB, tableName string) (bool, error) {
	// Query to check if the table exists in PostgreSQL
	query := `
        SELECT EXISTS (
            SELECT 1
            FROM   information_schema.tables
            WHERE  table_name=$1
        );
    `

	var exists bool

	err := db.QueryRow(query, tableName).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}
