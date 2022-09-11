package gcp

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"cloud.google.com/go/spanner"
	"github.com/golang/glog"
	"github.com/google/hashr/common"
	"google.golang.org/api/iterator"
	"google.golang.org/api/storage/v1"
	"google.golang.org/grpc/codes"
)

const (
	// Name contains name of the exporter.
	Name = "gcp"
)

// Exporter is an instance of GCP Exporter.
type Exporter struct {
	spannerClient  *spanner.Client
	storageClient  *storage.Service
	GCSBucket      string
	uploadPayloads bool
	workerCount    int
	wg             sync.WaitGroup
}

// NewExporter creates new GCP exporter.
func NewExporter(spannerClient *spanner.Client, storageClient *storage.Service, GCSBucket string, uploadPayloads bool, workerCount int) (*Exporter, error) {
	return &Exporter{spannerClient: spannerClient, storageClient: storageClient, GCSBucket: GCSBucket, uploadPayloads: uploadPayloads, workerCount: workerCount}, nil
}

// Name returns exporter name.
func (e *Exporter) Name() string {
	return Name
}

// Export exports extracted data to GCP (Spanner + GCS).
func (e *Exporter) Export(ctx context.Context, sourceRepoName, sourceRepoPath, sourceID, sourceHash, sourcePath, sourceDescription string, samples []common.Sample) error {
	if err := e.insertSource(ctx, sourceHash, sourceID, sourcePath, sourceRepoName, sourceRepoPath, sourceDescription); err != nil {
		return fmt.Errorf("could not upload source data: %v", err)
	}

	jobs := make(chan common.Sample, len(samples))
	for w := 1; w <= e.workerCount; w++ {
		e.wg.Add(1)
		go e.worker(ctx, sourceHash, jobs)
	}

	go func() {
		for _, sample := range samples {
			jobs <- sample
		}
		close(jobs)
	}()
	e.wg.Wait()

	return nil
}

func (e *Exporter) worker(ctx context.Context, sourceHash string, samples <-chan common.Sample) {
	defer e.wg.Done()
	for sample := range samples {
		if err := e.insertSample(ctx, sample); err != nil {
			glog.Errorf("skipping %s, could not insert sample data: %v", sample.Sha256, err)
			continue
		}

		if err := e.insertRelationship(ctx, sample, sourceHash); err != nil {
			glog.Errorf("skipping %s, could not insert source <-> sample relationship: %v", sample.Sha256, err)
			continue
		}
	}
}

func (e *Exporter) insertRelationship(ctx context.Context, sample common.Sample, sourceSha256 string) error {
	var paths, existingPaths []string

	for _, path := range sample.Paths {
		s := strings.Split(path, "/extracted/")
		if len(s) < 2 {
			glog.Warningf("sample path does not follow expected format: %s", path)
			continue
		}
		paths = append(paths, strings.TrimPrefix(strings.TrimPrefix(s[len(s)-1], "mnt"), "export"))
	}

	sql := spanner.Statement{
		SQL: `SELECT sample_paths FROM samples_sources WHERE sample_sha256 = @sha256`,
		Params: map[string]interface{}{
			"sha256": sample.Sha256,
		},
	}

	iter := e.spannerClient.Single().Query(ctx, sql)
	defer iter.Stop()
	row, err := iter.Next()
	if err != iterator.Done {
		if err := row.Columns(&existingPaths); err != nil {
			return err
		}
	}
	if err != iterator.Done && err != nil {
		return err
	}

	_, err = e.spannerClient.Apply(ctx, []*spanner.Mutation{
		spanner.InsertOrUpdate("samples_sources",
			[]string{
				"sample_sha256",
				"source_sha256",
				"sample_paths"},
			[]interface{}{
				sample.Sha256,
				sourceSha256,
				append(existingPaths, paths...),
			})})
	if err != nil {
		return fmt.Errorf("failed to insert data %v", err)
	}

	return nil
}

func (e *Exporter) insertSample(ctx context.Context, sample common.Sample) error {
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
	_, err = e.spannerClient.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("samples",
			[]string{
				"sha256",
				"mimetype",
				"file_output",
				"size"},
			[]interface{}{
				sample.Sha256,
				mimeType,
				fileOutput,
				fi.Size(),
			})})
	if spanner.ErrCode(err) != codes.AlreadyExists && err != nil {
		return fmt.Errorf("failed to insert data %v", err)
	}

	if e.uploadPayloads && sample.Upload {
		fmt.Println(samplePath)
		file, err := os.Open(samplePath)
		if err != nil {
			return fmt.Errorf("error while opening file: %v", err)
		}

		fi, err := file.Stat()
		if err != nil {
			return fmt.Errorf("error while opening file: %v", err)
		}

		fmt.Println(fi.Size())

		name := fmt.Sprintf("%s/%s", strings.ToUpper(sample.Sha256[0:2]), strings.ToUpper(sample.Sha256))
		object := &storage.Object{
			Name: name,
		}

		_, err = e.storageClient.Objects.Insert(e.GCSBucket, object).Media(file).Do()
		if err != nil {
			return fmt.Errorf("error uploading data to GCS: %v", err)
		}

		_, err = e.spannerClient.Apply(ctx, []*spanner.Mutation{
			spanner.Insert("payloads",
				[]string{
					"sha256",
					"gcs_path"},
				[]interface{}{
					sample.Sha256,
					fmt.Sprintf("gs://%s/%s", e.GCSBucket, name),
				})})
		if spanner.ErrCode(err) != codes.AlreadyExists && err != nil {
			return fmt.Errorf("failed to insert data %v", err)
		}
	}

	return nil
}

func (e *Exporter) insertSource(ctx context.Context, sourceHash, sourceID, sourcePath, sourceRepoName, sourceRepoPath, sourceDescription string) error {
	var sourceIDs []string

	sql := spanner.Statement{
		SQL: `SELECT source_id FROM sources WHERE sha256 = @sha256`,
		Params: map[string]interface{}{
			"sha256": sourceHash,
		},
	}

	iter := e.spannerClient.Single().Query(ctx, sql)
	defer iter.Stop()
	row, err := iter.Next()
	if err != iterator.Done {
		if err := row.Columns(&sourceIDs); err != nil {
			return err
		}
	}
	if err != iterator.Done && err != nil {
		return err
	}

	_, err = e.spannerClient.Apply(ctx, []*spanner.Mutation{
		spanner.InsertOrUpdate("sources",
			[]string{
				"sha256",
				"source_id",
				"source_path",
				"source_description",
				"repo_name",
				"repo_path"},
			[]interface{}{
				sourceHash,
				append(sourceIDs, sourceID),
				sourcePath,
				sourceDescription,
				sourceRepoName,
				sourceRepoPath,
			})})
	if err != nil {
		return fmt.Errorf("failed to insert data %v", err)
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
