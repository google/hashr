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

// Package cloudspanner implements cloud spanner as a hashR storage.
package cloudspanner

import (
	"context"
	"fmt"
	"time"

	"github.com/google/hashr/core/hashr"

	"cloud.google.com/go/spanner"

	"google.golang.org/api/iterator"
)

// Storage allows to interact with cloud spanner.
type Storage struct {
	spannerClient *spanner.Client
}

// NewStorage creates new Storage struct that allows to interact with cloud spanner.
func NewStorage(ctx context.Context, spannerClient *spanner.Client) (*Storage, error) {
	return &Storage{spannerClient: spannerClient}, nil
}

// UpdateJobs updates cloud spanner table.
func (s *Storage) UpdateJobs(ctx context.Context, qHash string, p *hashr.ProcessingSource) error {
	_, err := s.spannerClient.Apply(ctx, []*spanner.Mutation{
		spanner.InsertOrUpdate("jobs",
			[]string{
				"quick_sha256",
				"imported_at",
				"id",
				"repo",
				"repo_path",
				"location",
				"sha256",
				"status",
				"error",
				"preprocessing_duration",
				"processing_duration",
				"export_duration",
				"files_extracted",
				"files_exported"},
			[]interface{}{
				qHash,
				time.Unix(p.ImportedAt, 0),
				p.ID,
				p.Repo,
				p.RepoPath,
				p.RemoteSourcePath,
				p.Sha256,
				p.Status,
				p.Error,
				int64(p.PreprocessingDuration.Seconds()),
				int64(p.ProcessingDuration.Seconds()),
				int64(p.ExportDuration.Seconds()),
				p.SampleCount,
				p.ExportCount,
			})})
	if err != nil {
		return fmt.Errorf("failed to insert data %v", err)
	}

	return nil
}

// FetchJobs fetches processing jobs from cloud spanner.
func (s *Storage) FetchJobs(ctx context.Context) (map[string]string, error) {
	processed := make(map[string]string)
	iter := s.spannerClient.Single().Read(ctx, "jobs",
		spanner.AllKeys(), []string{"quick_sha256", "status"})
	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var quickSha256, status string
		err = row.ColumnByName("quick_sha256", &quickSha256)
		if err != nil {
			return nil, err
		}
		err = row.ColumnByName("status", &status)
		if err != nil {
			return nil, err
		}
		processed[quickSha256] = status
	}
	return processed, nil
}
