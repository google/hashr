package cloudspanner

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/glog"
	"github.com/google/hashr/common"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
)

// Exporter is an instance of Cloud Spanner Exporter.
type Exporter struct {
	spannerClient  *spanner.Client
	uploadPayloads bool
	workerCount    int
	wg             sync.WaitGroup
}

// NewStorage creates new Storage struct that allows to interact with cloud spanner.
func NewExporter(spannerClient *spanner.Client, uploadPayloads bool, workerCount int) (*Exporter, error) {
	return &Exporter{spannerClient: spannerClient, uploadPayloads: uploadPayloads, workerCount: workerCount}, nil
}

// Name returns exporter name.
func (e *Exporter) Name() string {
	return "cloudspanner"
}

// Export exports extracted data to Cloud Spanner instance.
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
		data, err := os.ReadFile(samplePath)
		if err != nil {
			return fmt.Errorf("error while opening file: %v", err)
		}

		_, err = e.spannerClient.Apply(ctx, []*spanner.Mutation{
			spanner.Insert("payloads",
				[]string{
					"sha256",
					"payload"},
				[]interface{}{
					sample.Sha256,
					data,
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
		SQL: `SELECT sourceID FROM sources WHERE sha256 = @sha256`,
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
				"sourceID",
				"sourcePath",
				"sourceDescription",
				"repoName",
				"repoPath"},
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
