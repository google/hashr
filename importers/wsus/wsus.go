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
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/common"

	"google.golang.org/api/storage/v1"
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
	updateTitle     string
}

// Preprocess extracts the contents of Windows Update file.
func (u *update) Preprocess() (string, error) {
	if err := u.download(); err != nil {
		return "", err
	}

	extractionDir := filepath.Join(u.tempDir, "extracted")
	if err := os.Mkdir(extractionDir, 0755); err != nil {
		return "", fmt.Errorf("could not create target %s directory: %v", extractionDir, err)
	}

	switch u.format {
	case cabArchive, exe:
		if err := u.recursiveExtract(extractionDir); err != nil {
			return "", err
		}
	}

	return extractionDir, nil
}

// ID returns non-unique Windows Update ID.
func (u *update) ID() string {
	return u.id
}

// RepoName returns repository name.
func (u *update) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (u *update) RepoPath() string {
	return fmt.Sprintf("gs://%s/", gcsBucket)
}

// LocalPath returns local path to a Windows Update file.
func (u *update) LocalPath() string {
	return u.localPath
}

// RemotePath returns remote path to a Windows Update file stored in the GCS bucket.
func (u *update) RemotePath() string {
	return fmt.Sprintf("gs://%s/%s", gcsBucket, u.remotePath)
}

// Description provides additional description for a Windows Update file.
func (u *update) Description() string {
	return u.updateTitle
}

// QuickSHA256Hash calculates sha256 hash of a Windows Update file metadata.
func (u *update) QuickSHA256Hash() (string, error) {
	if u.quickSha256hash != "" {
		return u.quickSha256hash, nil
	}

	u.quickSha256hash = fmt.Sprintf("%x", sha256.Sum256([]byte(u.md5hash)))

	return u.quickSha256hash, nil
}

func (u *update) download() error {
	_, file := filepath.Split(u.remotePath)

	resp, err := storageClient.Objects.Get(gcsBucket, u.remotePath).Download()
	if err != nil {
		glog.Exit(err)
		return err
	}
	defer resp.Body.Close()

	u.tempDir, err = common.LocalTempDir(u.ID())
	if err != nil {
		glog.Exit(err)
		return err
	}

	u.localPath = filepath.Join(u.tempDir, file)
	out, err := os.Create(u.localPath)
	if err != nil {
		glog.Exit(err)
		return fmt.Errorf("error while creating %s: %v", u.localPath, err)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("error while writing to %s: %v", u.localPath, err)
	}

	glog.Infof("Update %s successfully written to %s", u.remotePath, u.localPath)

	return nil
}

func (u *update) recursiveExtract(extractionDir string) error {
	if err := extract7z(u.localPath, extractionDir); err != nil {
		return err
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

// NewRepo returns new instance of a WSUS repository.
func NewRepo(ctx context.Context, storageService *storage.Service, gcsWSUSBucket string) (*Repo, error) {
	storageClient = storageService
	gcsBucket = gcsWSUSBucket
	return &Repo{}, nil
}

// Repo holds data related to a Windows WSUS repository.
type Repo struct {
	updates []*update
}

// RepoName returns repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (r *Repo) RepoPath() string {
	return fmt.Sprintf("gs://%s/", gcsBucket)
}

type wsusUpdate struct {
	filename     string
	kbArticle    string
	defaultTitle string
}

// DiscoverRepo traverses the repository and looks for files that are related to WSUS packages.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {
	updates, err := csvMapping()
	if err != nil {
		glog.Warningf("Could not get CSV mapping file: %v", err)
	}

	var token string
	for {
		resp, err := storageClient.Objects.List(gcsBucket).PageToken(token).Do()
		if err != nil {
			return nil, err
		}
		for _, obj := range resp.Items {
			_, filename := filepath.Split(obj.Name)
			ext := filepath.Ext(filename)
			sha1 := filename[0 : len(filename)-len(ext)]

			var id string
			var updateTitle string
			if val, exists := updates[sha1]; exists && val.filename != "" {
				id = val.filename
				updateTitle = val.defaultTitle
				if val.kbArticle != "" {
					updateTitle = fmt.Sprintf("KB%s %s", val.kbArticle, updateTitle)
				}
			} else {
				id = sha1
			}

			if strings.EqualFold(ext, ".cab") {
				r.updates = append(r.updates, &update{id: id, remotePath: obj.Name, md5hash: obj.Md5Hash, format: cabArchive, updateTitle: updateTitle})
			} else if strings.EqualFold(ext, ".exe") {
				r.updates = append(r.updates, &update{id: id, remotePath: obj.Name, md5hash: obj.Md5Hash, format: exe, updateTitle: updateTitle})
			}
		}
		token = resp.NextPageToken
		if token == "" {
			break
		}
	}

	var sources []hashr.Source
	for _, update := range r.updates {
		sources = append(sources, update)
	}

	return sources, nil
}

func csvMapping() (map[string]*wsusUpdate, error) {
	resp, err := storageClient.Objects.Get(gcsBucket, "export.csv").Download()
	if err != nil {
		return nil, fmt.Errorf("mapping file (%s) is not present", filepath.Join(gcsBucket, "export.csv"))
	}
	defer resp.Body.Close()

	var bodyBytes []byte
	if resp.StatusCode == http.StatusOK {
		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	}

	csvReader := csv.NewReader(bytes.NewReader(bodyBytes))
	csvReader.Comma = ';'
	rows, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("unable to parse CSV mapping file: %v", err)
	}

	updates := make(map[string]*wsusUpdate)

	for _, row := range rows[1:] {
		if val, exists := updates[row[0]]; exists {
			val.defaultTitle = fmt.Sprintf("%s, %s", val.defaultTitle, row[3])
		} else {
			updates[row[0]] = &wsusUpdate{filename: row[1], kbArticle: row[2], defaultTitle: row[3]}
		}
	}

	return updates, nil
}
