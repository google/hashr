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

package hashr

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/golang/glog"

	"github.com/google/hashr/common"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	database "cloud.google.com/go/spanner/admin/database/apiv1"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"

	dbadminpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	instancepb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
)

const (
	dbSchema = `CREATE TABLE jobs (
  imported_at TIMESTAMP NOT NULL,
  id STRING(500),
  repo STRING(200),
  repo_path STRING(500),
  quick_sha256 STRING(100) NOT NULL,
  location STRING(1000),
  sha256 STRING(100),
  status STRING(50),
  error STRING(10000),
  preprocessing_duration INT64,
  processing_duration INT64,
  export_duration INT64,
  files_extracted INT64,
  files_exported INT64,
) PRIMARY KEY(quick_sha256)`
)

type testImporter struct {
}

func (i *testImporter) RepoName() string {
	return "ubuntu"
}

func (i *testImporter) RepoPath() string {
	return "ubuntu"
}

func (i *testImporter) DiscoverRepo() ([]Source, error) {
	sources := []Source{
		&testSource{id: "001", localPath: "/tmp/001", quickSha256hash: "7a3e6b16cb75f48fb897eff3ae732f3154f6d203b53f33660f01b4c3b6bc2df9", repoPath: "/tmp/"},
		&testSource{id: "002", localPath: "/tmp/002", quickSha256hash: "a1dd6837f284625bdb1cb68f1dbc85c5dc4d8b05bae24c94ed5f55c477326ea2", repoPath: "/tmp/"},
	}

	for _, source := range sources {
		file, err := os.Create(source.RemotePath())
		if err != nil {
			return nil, fmt.Errorf("could not create dummy sources: %v", err)
		}
		file.Close()
	}

	return sources, nil
}

type testSource struct {
	id              string
	localPath       string
	quickSha256hash string
	repoPath        string
}

func (s *testSource) Preprocess() (string, error) {
	return "", nil
}
func (s *testSource) QuickSHA256Hash() (string, error) {
	return s.quickSha256hash, nil
}
func (s *testSource) RemotePath() string {
	return s.localPath
}
func (s *testSource) ID() string {
	return s.id
}
func (s *testSource) RepoName() string {
	return "ubuntu"
}
func (s *testSource) RepoPath() string {
	return "ubuntu"
}
func (s *testSource) Local() bool {
	return false
}
func (s *testSource) LocalPath() string {
	return s.localPath
}
func (s *testSource) Description() string {
	return ""
}

type testProcessor struct {
}

func (p *testProcessor) ImageExport(sourcePath string) (string, error) {
	return "testdata/20200106.00.00-ubuntu-laptop-export", nil
}

type testExporter struct {
}

func (e *testExporter) Export(ctx context.Context, repoName, repoPath, sourceID, sourceHash, sourcePath, sourceDescription string, samples []common.Sample) error {
	return nil
}

func (e *testExporter) Name() string {
	return "testExporter"
}

// TestRun requires Spanner emulator to be running: https://cloud.google.com/spanner/docs/emulator.
func TestRun(t *testing.T) {
	for _, tc := range []struct {
		export                bool
		exportPath            string
		exportWorkerCount     int
		processingWorkerCount int
		purgeJobsFile         bool
	}{
		{
			export:                false,
			exportPath:            "/tmp/hashr-export",
			exportWorkerCount:     100,
			processingWorkerCount: 1,
		},
		{
			export:                true,
			exportWorkerCount:     100,
			processingWorkerCount: 1,
			purgeJobsFile:         true,
		},
		{
			export:                false,
			exportPath:            "/tmp/hashr-export",
			processingWorkerCount: 1,
		},
		{
			export:                false,
			exportPath:            "/tmp/hashr-export",
			processingWorkerCount: 1,
		},
	} {
		ctx := context.Background()

		o := []option.ClientOption{
			option.WithEndpoint("localhost:9010"),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithInsecure()),
		}

		instanceAdmin, err := instance.NewInstanceAdminClient(ctx, o...)
		if err != nil {
			glog.Fatalf("error dialing instance admin: %v", err)
		}
		defer instanceAdmin.Close()

		if err := instanceAdmin.DeleteInstance(ctx, &instancepb.DeleteInstanceRequest{Name: "projects/hashr/instances/hashr"}); err != nil {
			glog.Warning(err)
		}

		op, err := instanceAdmin.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
			Parent:     "projects/hashr",
			InstanceId: "hashr",
			Instance: &instancepb.Instance{
				DisplayName: "hashr",
				NodeCount:   1,
			},
		})
		if err != nil {
			glog.Fatalf("error creating test spanner instance: %v", err)
		}
		if _, err := op.Wait(ctx); err != nil {
			glog.Fatalf("error creating test spanner instance: %v", err)
		}

		databaseAdmin, err := database.NewDatabaseAdminClient(ctx, o...)
		if err != nil {
			glog.Fatalf("error creating database admin client for emulator: %v", err)
		}

		dbURI := "projects/hashr/instances/hashr/databases/hashr"
		op2, err := databaseAdmin.CreateDatabase(ctx, &dbadminpb.CreateDatabaseRequest{
			Parent:          "projects/hashr/instances/hashr",
			CreateStatement: "CREATE DATABASE hashr",
			ExtraStatements: []string{dbSchema},
		})
		if err != nil {
			glog.Fatalf("error creating test DB %v: %v", dbURI, err)
		}
		if _, err = op2.Wait(ctx); err != nil {
			glog.Fatalf("error creating test DB %v: %v", dbURI, err)
		}

		spannerStorage, err := newStorage(ctx, dbURI, o...)
		if err != nil {
			glog.Fatalf("error creating test spanner client: %v", err)
		}

		hdb := New([]Importer{&testImporter{}}, &testProcessor{}, &testExporter{}, spannerStorage)
		hdb.CacheDir = "/tmp/"
		hdb.Export = tc.export
		hdb.ExportPath = tc.exportPath
		hdb.ExportWorkerCount = tc.exportWorkerCount
		hdb.ProcessingWorkerCount = tc.processingWorkerCount

		// This is a simple test to check the full processing logic with different number of workers.
		// The test should fail on any error.
		// TODO(mlegin): Add a test to check the telemetry stats of the whole Run.
		for i := 1; i <= tc.processingWorkerCount; i++ {
			hdb.ProcessingWorkerCount = i
			if err := hdb.Run(context.Background()); err != nil {
				t.Errorf("Unexpected error while running hashR: %v", err)
			}
		}
	}
}

// Storage allows to interact with cloud spanner.
type fakeStorage struct {
	spannerClient *spanner.Client
}

// NewStorage creates new Storage struct that allows to interact with cloud spanner.
func newStorage(ctx context.Context, spannerDBPath string, opts ...option.ClientOption) (*fakeStorage, error) {
	spannerClient, err := spanner.NewClient(ctx, spannerDBPath, opts...)
	if err != nil {
		return nil, err
	}

	return &fakeStorage{spannerClient: spannerClient}, nil
}

// UpdateJobs updates cloud spanner table.
func (s *fakeStorage) UpdateJobs(ctx context.Context, qHash string, p *ProcessingSource) error {
	return nil
}

// FetchJobs fetches processing jobs from cloud spanner.
func (s *fakeStorage) FetchJobs(ctx context.Context) (map[string]string, error) {
	return make(map[string]string), nil
}
