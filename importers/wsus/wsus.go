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

// Package wsus implements WSUS repository importer.
package wsus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/common"

	storage "google.golang.org/api/storage/v1"
)

type updateFormat int

const (
	// RepoName contains the repository name.
	RepoName              = "wsus"
	path7z                = "/usr/bin/7z"
	exe      updateFormat = iota
	cabArchive
)

var (
	storageClient = &storage.Service{}
	gcsBucket     string
)

type update struct {
	id              string
	remotePath      string
	md5hash         string
	quickSha256hash string
	localPath       string
	tempDir         string
	format          updateFormat
}

// Preprocess extracts the contents of Windows Update file.
func (i *update) Preprocess() (string, error) {
	if err := i.download(); err != nil {
		return "", err
	}

	extractionDir := filepath.Join(i.tempDir, "extracted")
	if err := os.Mkdir(extractionDir, 0755); err != nil {
		return "", fmt.Errorf("could not create target %s directory: %v", extractionDir, err)
	}

	switch i.format {
	case cabArchive, exe:
		if err := i.recursiveExtract(extractionDir); err != nil {
			return "", err
		}
	}

	return extractionDir, nil
}

// ID returns non-unique Windows Update ID.
func (i *update) ID() string {
	return i.id
}

// RepoName returns repository name.
func (i *update) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (i *update) RepoPath() string {
	return fmt.Sprintf("gs://%s/", gcsBucket)
}

// LocalPath returns local path to a Windows Update file.
func (i *update) LocalPath() string {
	return i.localPath
}

// RemotePath returns remote path to a Windows Update file stored in the GCS bucket.
func (i *update) RemotePath() string {
	return fmt.Sprintf("gs://%s/%s", gcsBucket, i.remotePath)
}

// QuickSHA256Hash calculates sha256 hash of a Windows Update file metadata.
func (i *update) QuickSHA256Hash() (string, error) {
	if i.quickSha256hash != "" {
		return i.quickSha256hash, nil
	}

	i.quickSha256hash = fmt.Sprintf("%x", sha256.Sum256([]byte(i.id+i.md5hash)))

	return i.quickSha256hash, nil
}

func (i *update) download() error {
	_, file := filepath.Split(i.remotePath)

	resp, err := storageClient.Objects.Get(gcsBucket, i.remotePath).Download()
	if err != nil {
		glog.Exit(err)
		return err
	}
	defer resp.Body.Close()

	i.tempDir, err = common.LocalTempDir(i.ID())
	if err != nil {
		glog.Exit(err)
		return err
	}

	i.localPath = filepath.Join(i.tempDir, file)
	out, err := os.Create(i.localPath)
	if err != nil {
		glog.Exit(err)
		return fmt.Errorf("error while creating %s: %v", i.localPath, err)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("error while writing to %s: %v", i.localPath, err)
	}

	glog.Infof("Update %s successfully written to %s", i.remotePath, i.localPath)

	return nil
}

func (i *update) recursiveExtract(extractionDir string) error {
	if err := extract7z(i.localPath, extractionDir); err != nil {
		return err
	}

	// If the manifest file exists, try to extract KB ID.
	manifestPath := filepath.Join(extractionDir, "_manifest_.cix.xml")
	if _, err := os.Stat(manifestPath); err == nil {
		kbID, err := extractKB(manifestPath)
		if err != nil {
			glog.Errorf("could not extract KB ID: %v", err)
		}
		i.id = fmt.Sprintf("%s-%s", i.id, kbID)
	}

	var files []string
	if err := filepath.Walk(extractionDir, walk(&files)); err != nil {
		return err
	}

	if err := recursiveExtract(files); err != nil {
		return err
	}

	return nil
}

func extractFile(fileCmdOutput string) bool {
	toBeExtracted := []string{"compressed", "archive", "application/x-msi", "application/vnd.ms-msi"}
	for _, s := range toBeExtracted {
		if strings.Contains(fileCmdOutput, s) {
			return true
		}
	}

	return false
}

func extractExt(ext string) bool {
	toBeExtracted := []string{".cab", ".msi", ".7z", ".msu", ".msp"}
	for _, e := range toBeExtracted {
		if strings.EqualFold(e, ext) {
			return true
		}
	}
	return false
}

func recursiveExtract(files []string) error {
	for _, fullPath := range files {
		_, filename := filepath.Split(fullPath)
		extension := filepath.Ext(filename)

		var tryToExtract bool
		// Since the files come from MS we can trust the extension, but if there is none, we should
		// try to detect the mime type. The easiest and most reliable solution is to run file binary.
		if extension == "" {
			out, err := shellCommand("/usr/bin/file", "--mime-type", fullPath)
			if err != nil {
				return err
			}
			if extractFile(out) {
				tryToExtract = true
			}
		}

		if extractExt(extension) || tryToExtract {
			targetDir := fmt.Sprintf("%s_%s", fullPath, "extracted")
			if err := os.Mkdir(targetDir, 0755); err != nil {
				return fmt.Errorf("could not create target %s directory: %v", targetDir, err)
			}
			if err := extract7z(fullPath, targetDir); err != nil {
				glog.Errorf("error while extracting archive file: %v", err)
				continue
			}

			var extractedFiles []string
			if err := filepath.Walk(targetDir, walk(&extractedFiles)); err != nil {
				return err
			}

			if len(extractedFiles) > 0 {
				if err := recursiveExtract(extractedFiles); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// This is mainly to enable testing.
var execute = func(name string, args ...string) *exec.Cmd {
	glog.Infof("name: %v, args: %v", name, args)
	return exec.Command(name, args...)
}

func shellCommand(binary string, args ...string) (string, error) {
	cmd := execute(binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error while executing %s: %v\nStdout: %v\nStderr: %v", binary, err, stdout.String(), stderr.String())
	}

	return stdout.String(), nil
}

func extract7z(filepath, targetDir string) error {
	_, err := shellCommand(path7z, "x", filepath, fmt.Sprintf("-o%s", targetDir))
	if err != nil {
		return fmt.Errorf("error while running 7z: %v", err)
	}

	return nil
}

func walk(files *[]string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			glog.Errorf("Could not open %s: %v", path, err)
			return nil
		}
		if info.IsDir() {
			return nil
		}

		*files = append(*files, path)
		return nil
	}
}

func extractKB(manifestPath string) (string, error) {
	xmlFile, err := os.Open(manifestPath)
	if err != nil {
		return "", fmt.Errorf("error while opening XML manifest file: %v", err)
	}

	defer xmlFile.Close()

	type Update struct {
		XMLName xml.Name `xml:"Container"`
		Name    string   `xml:"name,attr"`
	}

	byteValue, err := ioutil.ReadAll(xmlFile)
	if err != nil {
		return "", fmt.Errorf("error while reading XML manifest file: %v", err)
	}

	var update Update
	err = xml.Unmarshal(byteValue, &update)
	if err != nil {
		return "", fmt.Errorf("error while unmarshalling XML manifest file: %v", err)
	}

	return update.Name, nil
}

// NewRepo returns new instance of a WSUS repository.
func NewRepo(ctx context.Context, storageService *storage.Service, gcsWSUSBucket string) (*Repo, error) {
	storageClient = storageService
	gcsBucket = gcsWSUSBucket
	return &Repo{}, nil
}

// Repo holds data related to a Windows WSUS repository.
type Repo struct {
	name   string
	files  []string
	images []*update
}

// RepoName returns repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (r *Repo) RepoPath() string {
	return fmt.Sprintf("gs://%s/", gcsBucket)
}

// DiscoverRepo traverses the repository and looks for files that are related to WSUS packages.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {
	var token string

	for {
		resp, err := storageClient.Objects.List(gcsBucket).PageToken(token).Do()
		if err != nil {
			return nil, err
		}
		for _, obj := range resp.Items {
			_, filename := filepath.Split(obj.Name)
			ext := filepath.Ext(filename)
			name := filename[0 : len(filename)-len(ext)]
			if strings.EqualFold(ext, ".cab") {
				r.images = append(r.images, &update{id: name, remotePath: obj.Name, md5hash: obj.Md5Hash, format: cabArchive})
			} else if strings.EqualFold(ext, ".exe") {
				r.images = append(r.images, &update{id: name, remotePath: obj.Name, md5hash: obj.Md5Hash, format: exe})
			}
		}
		token = resp.NextPageToken
		if token == "" {
			break
		}
	}

	var sources []hashr.Source
	for _, image := range r.images {
		sources = append(sources, image)
	}

	return sources, nil
}
