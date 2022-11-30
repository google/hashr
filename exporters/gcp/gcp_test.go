package gcp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/golang/glog"
	"github.com/google/hashr/common"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"

	dbadminpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	instancepb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
)

const (
	samplesTable = `
	CREATE TABLE samples (
		sha256 STRING(100),
		mimetype STRING(MAX),
		file_output  STRING(MAX),
		size INT64
	) PRIMARY KEY(sha256)`

	payloadsTable = `
	CREATE TABLE payloads (
		sha256 STRING(100),
		gcs_path STRING(200)
	) PRIMARY KEY(sha256)`

	sourcesTable = `
	CREATE TABLE sources (
		sha256 STRING(100),
        source_id  ARRAY<STRING(MAX)>,
        source_path STRING(MAX),
        source_description STRING(MAX),
        repo_name STRING(MAX),
        repo_path STRING(MAX),
	) PRIMARY KEY(sha256)`

	samplesSourcesTable = `CREATE TABLE samples_sources (
		sample_sha256 STRING(100),
		source_sha256 STRING(100),
		sample_paths ARRAY<STRING(MAX)>,
		CONSTRAINT FK_Sample FOREIGN KEY (sample_sha256) REFERENCES samples (sha256),
		CONSTRAINT FK_Source FOREIGN KEY (source_sha256) REFERENCES sources (sha256),
	)  PRIMARY KEY (sample_sha256, source_sha256)`
)

func TestExport(t *testing.T) {
	ctx := context.Background()

	o := []option.ClientOption{
		option.WithEndpoint("localhost:9010"),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
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
		ExtraStatements: []string{samplesTable, sourcesTable, payloadsTable, samplesSourcesTable},
	})
	if err != nil {
		glog.Fatalf("error creating test DB %v: %v", dbURI, err)
	}
	if _, err = op2.Wait(ctx); err != nil {
		glog.Fatalf("error creating test DB %v: %v", dbURI, err)
	}

	spannerClient, err := spanner.NewClient(ctx, dbURI, o...)
	if err != nil {
		glog.Fatalf("error creating Spanner client %v: %v", dbURI, err)
	}

	exporter, err := NewExporter(spannerClient, nil, "gcs-bucket", false, 10)
	if err != nil {
		glog.Fatalf("error creating Cloud Spanner exporter: %v", err)
	}

	samples := []common.Sample{
		{
			Sha256: "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
			Paths:  []string{filepath.Join("testdata/extraction", "file.01")},
			Upload: true,
		},
		{
			Sha256: "5c7a0f6e38f86f4db12130e5ca9f734f4def519b9a884ee8ea9fc45f9626c6fb",
			Paths:  []string{filepath.Join("testdata/extraction", "file.02")},
			Upload: true,
		},
		{
			Sha256: "9ad2027cae0d7b0f041a6fc1e3124ad4046b2665068c44c74546ad9811e81ec7",
			Paths:  []string{filepath.Join("testdata/extraction", "file.03")},
			Upload: true,
		},
	}

	if err := exporter.Export(ctx, "GCP", "ubuntu", "ubuntu-1604-lts", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc", "", "Official Ubuntu GCP image.", samples); err != nil {
		t.Fatalf("unexpected error while running Export() = %v", err)
	}

}
