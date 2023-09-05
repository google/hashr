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

package postgres

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/hashr/common"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestExport(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("could not open a stub database connection: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT EXISTS ( SELECT 1 FROM information_schema.tables WHERE table_name=$1 );`).WithArgs("samples").WillReturnRows(mock.NewRows([]string{"t"}).AddRow("t"))
	mock.ExpectQuery(`SELECT EXISTS ( SELECT 1 FROM information_schema.tables WHERE table_name=$1 );`).WithArgs("payloads").WillReturnRows(mock.NewRows([]string{"t"}).AddRow("t"))
	mock.ExpectQuery(`SELECT EXISTS ( SELECT 1 FROM information_schema.tables WHERE table_name=$1 );`).WithArgs("sources").WillReturnRows(mock.NewRows([]string{"t"}).AddRow("t"))
	mock.ExpectQuery(`SELECT EXISTS ( SELECT 1 FROM information_schema.tables WHERE table_name=$1 );`).WithArgs("samples_sources").WillReturnRows(mock.NewRows([]string{"t"}).AddRow("t"))

	postgresExporter, err := NewExporter(db, false)
	if err != nil {
		t.Fatalf("could not create Postgres exporter: %v", err)
	}

	mock.ExpectQuery(`SELECT sha256 FROM sources WHERE sha256=$1;`).WithArgs("07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc").WillReturnRows(mock.NewRows([]string{"sha256"}))
	mock.ExpectExec(`INSERT INTO sources (sha256, sourceID, sourcePath, repoName, repoPath, sourceDescription) VALUES ($1, $2, $3, $4, $5, $6)`).WithArgs("07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc", `{"ubuntu-1604-lts"}`, "", "GCP", "ubuntu", "Official Ubuntu GCP image.").WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(`SELECT sha256 FROM samples WHERE sha256=$1;`).WithArgs("a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3").WillReturnRows(mock.NewRows([]string{"sha256"}))
	mock.ExpectExec(`INSERT INTO samples (sha256, size, mimetype, file_output) VALUES ($1, $2, $3, $4)`).WithArgs("a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3", 8192, "application/octet-stream", " data").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT sample_sha256,source_sha256 FROM samples_sources WHERE sample_sha256=$1 AND source_sha256=$2;").WithArgs("a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc").WillReturnRows(mock.NewRows([]string{"a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc"}))
	mock.ExpectExec(`INSERT INTO samples_sources (sample_sha256, source_sha256, sample_paths) VALUES ($1, $2, $3)`).WithArgs("a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc", `{"file.01"}`).WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(`SELECT sha256 FROM samples WHERE sha256=$1;`).WithArgs("5c7a0f6e38f86f4db12130e5ca9f734f4def519b9a884ee8ea9fc45f9626c6fb").WillReturnRows(mock.NewRows([]string{"sha256"}))
	mock.ExpectExec(`INSERT INTO samples (sha256, size, mimetype, file_output) VALUES ($1, $2, $3, $4)`).WithArgs("5c7a0f6e38f86f4db12130e5ca9f734f4def519b9a884ee8ea9fc45f9626c6fb", 7168, "application/octet-stream", " data").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT sample_sha256,source_sha256 FROM samples_sources WHERE sample_sha256=$1 AND source_sha256=$2;").WithArgs("5c7a0f6e38f86f4db12130e5ca9f734f4def519b9a884ee8ea9fc45f9626c6fb", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc").WillReturnRows(mock.NewRows([]string{"5c7a0f6e38f86f4db12130e5ca9f734f4def519b9a884ee8ea9fc45f9626c6fb", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc"}))
	mock.ExpectExec(`INSERT INTO samples_sources (sample_sha256, source_sha256, sample_paths) VALUES ($1, $2, $3)`).WithArgs("5c7a0f6e38f86f4db12130e5ca9f734f4def519b9a884ee8ea9fc45f9626c6fb", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc", `{"file.02"}`).WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(`SELECT sha256 FROM samples WHERE sha256=$1;`).WithArgs("9ad2027cae0d7b0f041a6fc1e3124ad4046b2665068c44c74546ad9811e81ec7").WillReturnRows(mock.NewRows([]string{"sha256"}))
	mock.ExpectExec(`INSERT INTO samples (sha256, size, mimetype, file_output) VALUES ($1, $2, $3, $4)`).WithArgs("9ad2027cae0d7b0f041a6fc1e3124ad4046b2665068c44c74546ad9811e81ec7", 5120, "application/octet-stream", " data").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT sample_sha256,source_sha256 FROM samples_sources WHERE sample_sha256=$1 AND source_sha256=$2;").WithArgs("9ad2027cae0d7b0f041a6fc1e3124ad4046b2665068c44c74546ad9811e81ec7", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc").WillReturnRows(mock.NewRows([]string{"9ad2027cae0d7b0f041a6fc1e3124ad4046b2665068c44c74546ad9811e81ec7", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc"}))
	mock.ExpectExec(`INSERT INTO samples_sources (sample_sha256, source_sha256, sample_paths) VALUES ($1, $2, $3)`).WithArgs("9ad2027cae0d7b0f041a6fc1e3124ad4046b2665068c44c74546ad9811e81ec7", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc", `{"file.03"}`).WillReturnResult(sqlmock.NewResult(1, 1))

	tempDir := "/tmp/extracted/"
	if err := os.MkdirAll(tempDir, 0777); err != nil {
		t.Fatalf("Could not create temp extraction directory(%s): %v", tempDir, err)
	}

	// We need to copy the file to the tmp dir, otherwise we'll end up opening symlinks.
	for _, filename := range []string{"file.01", "file.02", "file.03"} {
		in, err := os.Open(filepath.Join("testdata/extraction", filename))
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.Create(filepath.Join(tempDir, filename))
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(out, in)
		if err != nil {
			t.Fatal(err)
		}
		out.Close()
	}

	samples := []common.Sample{
		{
			Sha256: "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3",
			Paths:  []string{filepath.Join(tempDir, "file.01")},
			Upload: true,
		},
		{
			Sha256: "5c7a0f6e38f86f4db12130e5ca9f734f4def519b9a884ee8ea9fc45f9626c6fb",
			Paths:  []string{filepath.Join(tempDir, "file.02")},
			Upload: true,
		},
		{
			Sha256: "9ad2027cae0d7b0f041a6fc1e3124ad4046b2665068c44c74546ad9811e81ec7",
			Paths:  []string{filepath.Join(tempDir, "file.03")},
			Upload: true,
		},
	}

	if err := postgresExporter.Export(context.Background(), "GCP", "ubuntu", "ubuntu-1604-lts", "07123e1f482356c415f684407a3b8723e10b2cbbc0b8fcd6282c49d37c9c1abc", "", "Official Ubuntu GCP image.", samples); err != nil {
		t.Fatalf("unexpected error while running Export() = %v", err)
	}
}
