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

// Package hashr implements core functionality for hashR.
package hashr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/hashr/cache"
	"github.com/google/hashr/common"
)

// Source represents data to be processed.
type Source interface {
	// ID returns non-unique ID for a given source.
	ID() string
	// RepoName returns source repository name.
	RepoName() string
	// RepoPath returns source repository path.
	RepoPath() string
	// LocalPath returns path to the source on the local file system.
	LocalPath() string
	// RemotePath returns path to the source in the remote location.
	RemotePath() string
	// Preprocess does the necessary preprocessing (extracting, mounting) to ingest the data in
	// Plaso.
	Preprocess() (string, error)
	// QuickSHA256Hash returns SHA256 digest, that is used to check if a given source data was
	// already processed.Given the fact that some repositories will hold a lot of data, the intent
	// here is to use the least resource demanding method to return a source digest.
	QuickSHA256Hash() (string, error)
}

// Importer represents importer instance that will be used to import data for processing.
type Importer interface {
	// DiscoverRepo returns slice of objects that satisfy Source interface.
	DiscoverRepo() ([]Source, error)
	// RepoName() returns repository name.
	RepoName() string
	// RepoPath() returns repository path.
	RepoPath() string
}

// Processor represents processor instance that will be used to process source data.
type Processor interface {
	// ImageExport runs image_export.py binary and returns local path to the folder with extracted
	// data.
	ImageExport(string) (string, error)
}

// Storage represents  storage that is used to store data about processed sources.
type Storage interface {
	UpdateJobs(ctx context.Context, qHash string, p *ProcessingSource) error
	FetchJobs(ctx context.Context) (map[string]string, error)
}

// Exporter represents exporter instance that will be used to export extracted data.
type Exporter interface {
	// Export exports samples to a given data sink.
	Export(ctx context.Context, repoName, repoPath, sourceID, sourceHash, sourcePath string, samples []common.Sample) error
	// Name returns exporter name.
	Name() string
}

// HashR holds data related to running instance of HashR.
type HashR struct {
	Importers              []Importer
	Processor              Processor
	Exporter               Exporter
	Storage                Storage
	ProcessingWorkerCount  int
	ExportWorkerCount      int
	CacheDir               string
	Dev                    bool
	Export                 bool
	ExportPath             string
	SourcesForReprocessing []string
	cacheSaveCounter       int
	wg                     sync.WaitGroup
	mu                     sync.Mutex
	processingSources      map[string]*ProcessingSource
}

// ProcessingSource holds data related to a processing source.
type ProcessingSource struct {
	ID                    string
	Repo                  string
	RepoPath              string
	RemoteSourcePath      string
	Sha256                string
	Status                status
	ImportedAt            int64
	PreprocessingDuration time.Duration
	ProcessingDuration    time.Duration
	ExportDuration        time.Duration
	SampleCount           int
	ExportCount           int
	Error                 string
}

// Status is a type to store the status of a processing job.
type status string

const (
	discovered   = "discovered"
	preprocessed = "preprocessed"
	processed    = "processed"
	cached       = "cached"
	exported     = "exported"
	failed       = "failed"
	reprocess    = "reprocess"
)

// New returns new instance of hashR.
func New(importers []Importer, processor Processor, exporter Exporter, storage Storage) *HashR {
	return &HashR{Importers: importers, Processor: processor, Exporter: exporter, Storage: storage}
}

// newSources returns sources that were not yet processed.
func (h *HashR) newSources(ctx context.Context, i Importer) ([]Source, error) {
	var newSources []Source

	glog.Infof("Discovering %s %s repository.", i.RepoName(), i.RepoPath())
	sources, err := i.DiscoverRepo()
	if err != nil {
		return nil, fmt.Errorf("%s: error discovering repo: %v", i.RepoName(), err)
	}
	glog.Infof("Discovered %d sources in %s repository.", len(sources), i.RepoName())

	processedSources, err := h.Storage.FetchJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not fetch processed sources from storage: %v", err)
	}

	for _, source := range sources {
		qHash, err := source.QuickSHA256Hash()
		if err != nil {
			glog.Errorf("%s: skipping source due to quick hashing error: %v", source.ID(), err)
			continue
		}
		glog.Infof("Discovered source: %s, with quick SHA256: %s", source.ID(), qHash)
		// Check if the source was already processed or should be reprocessed.

		status, processed := processedSources[qHash]
		if !processed || contains(h.SourcesForReprocessing, qHash) || strings.EqualFold(status, reprocess) {
			newSources = append(newSources, source)
		}
	}
	glog.Infof("Discovered %d new sources in %s (%s) repository.", len(newSources), i.RepoName(), i.RepoPath())

	return newSources, nil
}

func contains(slice []string, s string) bool {
	for _, element := range slice {
		if strings.EqualFold(s, element) {
			return true
		}
	}
	return false
}

// process executes functions required to export files using image_export.py
func (h *HashR) process(source Source, processor Processor) (*common.Extraction, error) {
	qhash, err := source.QuickSHA256Hash()
	if err != nil {
		glog.Errorf("%s: skipping source due to quick hashing error: %v", source.ID(), err)
	}

	start := time.Now()

	glog.Infof("Preprocessing %s", source.ID())
	plasoInput, err := source.Preprocess()
	if err != nil {
		return &common.Extraction{}, fmt.Errorf("error while preprocessing: %v", err)
	}

	extraction := &common.Extraction{SourceID: source.ID(), RepoName: source.RepoName()}
	extraction.BaseDir, _ = filepath.Split(source.LocalPath())
	glog.Infof("Done preprocessing %s", source.LocalPath())
	h.processingSources[qhash].PreprocessingDuration = time.Since(start)
	start = time.Now()
	glog.Infof("Calculating SHA256 of %s", source.LocalPath())
	extraction.SourceSHA256, err = sha256sum(source.LocalPath())
	if err != nil {
		return extraction, fmt.Errorf("error while hashing: %v", err)
	}
	glog.Infof("SHA256(%s) = %s", source.LocalPath(), extraction.SourceSHA256)

	extraction.Path, err = processor.ImageExport(plasoInput)
	if err != nil {
		return extraction, fmt.Errorf("error while processing: %v", err)
	}
	glog.Infof("Done processing %s", source.LocalPath())
	h.processingSources[qhash].ProcessingDuration = time.Since(start)

	return extraction, nil
}

func cleanupLocalStorage(path string) error {
	glog.Infof("Deleting %s", path)

	cmd := exec.Command("sudo", "rm", "-rf", path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if strings.HasPrefix(path, "/tmp/hashr") {
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("error while trying to remove %s: %v\nStdout: %v\nStderr: %v", path, err, stdout.String(), stderr.String())
		}
	}

	return nil
}

func sha256sum(path string) (string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

// Run executes main processing loop for hashR.
func (h *HashR) Run(ctx context.Context) error {
	h.processingSources = make(map[string]*ProcessingSource)
	h.cacheSaveCounter = 0

	for _, importer := range h.Importers {
		newSources, err := h.newSources(ctx, importer)
		if err != nil {
			glog.Errorf("skipping %s repo: %v", importer.RepoName(), err)
			continue
		}

		if len(newSources) == 0 {
			glog.Infof("No new sources in %s (%s) repo.", importer.RepoName(), importer.RepoPath())
			continue
		}

		c, err := cache.Load(importer.RepoName(), h.CacheDir)
		if err != nil {
			glog.Errorf("skipping %s repo: %v", importer.RepoName(), err)
			continue
		}

		processingJobs := make(chan Source)
		for w := 1; w <= h.ProcessingWorkerCount; w++ {
			h.wg.Add(1)
			go h.processingWorker(ctx, processingJobs, c)
		}

		go func() {
			for _, newSource := range newSources {
				processingJobs <- newSource
			}
			close(processingJobs)
		}()

		h.wg.Wait()

		err = cache.Save(importer.RepoName(), h.CacheDir, c)
		if err != nil {
			glog.Errorf("could not save %s repo cache: %v", importer.RepoName(), err)
		}
	}

	return nil
}

func (h *HashR) handleError(ctx context.Context, quickHash, extractionBaseDir string, processingSource *ProcessingSource, err error) {
	glog.Errorf("%s: skipping source %s: %v", processingSource.Repo, processingSource.ID, err)
	processingSource.Status = failed
	processingSource.Error = err.Error()
	h.processingSources[quickHash].Error = err.Error()
	if err := h.Storage.UpdateJobs(ctx, quickHash, h.processingSources[quickHash]); err != nil {
		glog.Errorf("could not update storage: %v", err)
	}

	if err = cleanupLocalStorage(extractionBaseDir); err != nil {
		glog.Errorf("could not clean-up local storage at %s: %v", extractionBaseDir, err)
	}
}

func (h *HashR) processingWorker(ctx context.Context, newSources <-chan Source, c *sync.Map) error {
	defer h.wg.Done()
	for source := range newSources {
		qHash, err := source.QuickSHA256Hash()
		if err != nil {
			glog.Errorf("%s: skipping source %s, could not calculate quick sha256 value: %v", source.RepoName(), source.ID(), err)
			continue
		}

		h.processingSources[qHash] = &ProcessingSource{Repo: source.RepoName(), RepoPath: source.RepoPath(), ID: source.ID(), RemoteSourcePath: source.RemotePath(), ImportedAt: time.Now().Unix(), Status: discovered}

		if err := h.Storage.UpdateJobs(ctx, qHash, h.processingSources[qHash]); err != nil {
			glog.Errorf("could not update storage: %v", err)
		}

		extraction, err := h.process(source, h.Processor)
		if err != nil {
			h.handleError(ctx, qHash, extraction.BaseDir, h.processingSources[qHash], err)
			continue
		}

		h.processingSources[qHash].Sha256 = extraction.SourceSHA256
		h.processingSources[qHash].Status = processed
		if err := h.Storage.UpdateJobs(ctx, qHash, h.processingSources[qHash]); err != nil {
			glog.Errorf("could not update storage: %v", err)
		}

		glog.Infof("Checking cache for existing samples from %s", source.ID())

		samples, err := cache.Check(extraction, c)
		if err != nil {
			h.handleError(ctx, qHash, extraction.BaseDir, h.processingSources[qHash], err)
			continue
		}

		glog.Infof("Done checking cache for existing samples from %s", source.ID())

		h.processingSources[qHash].Status = cached
		if err := h.Storage.UpdateJobs(ctx, qHash, h.processingSources[qHash]); err != nil {
			glog.Errorf("could not update storage: %v", err)
		}

		h.mu.Lock()
		// This is to avoid saving cache file (which can be more than 10GB in size) every ~5min.
		if h.cacheSaveCounter > 20 {
			glog.Infof("Saving cache after processing %s", source.ID())
			err = cache.Save(source.RepoName(), h.CacheDir, c)
			if err != nil {
				glog.Errorf("could not save %s repo cache: %v", source.RepoName(), err)
			}
			glog.Infof("Done saving cache after processing %s", source.ID())
			h.cacheSaveCounter = 0
		}

		// TODO(mlegin): Iterate over all definied exporters.
		if h.Export {
			start := time.Now()
			glog.Infof("Exporting samples from %s with %s hash", source.ID(), extraction.SourceSHA256)
			err = h.Exporter.Export(ctx, source.RepoName(), source.RepoPath(), extraction.SourceID, extraction.SourceSHA256, source.RemotePath(), samples)
			if err != nil {
				h.handleError(ctx, qHash, extraction.BaseDir, h.processingSources[qHash], err)
				h.mu.Unlock()
				continue
			}
			h.processingSources[qHash].ExportDuration = time.Since(start)
			h.processingSources[qHash].SampleCount = len(samples)
			for _, sample := range samples {
				if sample.Upload {
					h.processingSources[qHash].ExportCount++
				}
			}
		} else {
			err = h.saveSamples(source.RepoName(), extraction.SourceID, extraction.SourceSHA256, samples)
			if err != nil {
				h.handleError(ctx, qHash, extraction.BaseDir, h.processingSources[qHash], err)
				h.mu.Unlock()
				continue
			}
		}

		glog.Infof("Done exporting samples from %s with %s hash", source.ID(), extraction.SourceSHA256)

		h.processingSources[qHash].Status = exported
		if err := h.Storage.UpdateJobs(ctx, qHash, h.processingSources[qHash]); err != nil {
			glog.Errorf("could not update storage: %v", err)
		}

		if err = cleanupLocalStorage(extraction.BaseDir); err != nil {
			glog.Errorf("could not clean-up local storage at %s: %v", extraction.BaseDir, err)
		}

		h.mu.Unlock()
		h.cacheSaveCounter++
	}

	return nil
}

func (h *HashR) saveSamples(sourceImporter, sourceID, sourceHash string, samples []common.Sample) error {
	var samplesOut []common.Sample
	subDir := fmt.Sprintf("%s___%s___%s", sourceImporter, sourceID, sourceHash)
	destDir := filepath.Join(h.ExportPath, subDir)
	glog.Infof("Saving samples locally to %s", destDir)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, sample := range samples {
		if !sample.Upload {
			samplesOut = append(samplesOut, common.Sample{Sha256: sample.Sha256, Upload: false})
		} else {
			if err := os.Mkdir(filepath.Join(destDir, sample.Sha256), 0755); err != nil {
				return err
			}

			var samplePath string
			// If sample has more than one path associated with it, take the first that is valid.
			for _, path := range sample.Paths {
				if _, err := os.Stat(path); err == nil {
					samplePath = path
					break
				}
			}

			input, err := ioutil.ReadFile(samplePath)
			if err != nil {
				return err
			}

			destFile := filepath.Join(destDir, sample.Sha256, filepath.Base(samplePath))

			err = ioutil.WriteFile(destFile, input, 0755)
			if err != nil {
				return err
			}

			samplesOut = append(samplesOut, common.Sample{Sha256: sample.Sha256, Paths: []string{destFile}, Upload: true})
		}
	}

	jsonBytes, err := json.Marshal(samplesOut)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(destDir, "samples.json"), jsonBytes, 0755)
	if err != nil {
		return err
	}

	return nil
}
