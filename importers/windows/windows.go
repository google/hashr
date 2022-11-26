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

// Package windows implements Windows ISO-13346 repository importer.
package windows

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Microsoft/go-winio/wim"
	"github.com/golang/glog"
	"github.com/google/hashr/core/hashr"
	"github.com/google/hashr/importers/common"
)

const (
	// RepoName contains the repository name.
	RepoName = "windows"
)

// Preprocess extracts the contents of Windows ISO file.
func (w *wimImage) Preprocess() (string, error) {
	var err error
	w.localPath, err = common.CopyToLocal(w.remotePath, w.id)
	if err != nil {
		return "", fmt.Errorf("error while copying %s to %s: %v", w.remotePath, w.localPath, err)
	}

	baseDir, _ := filepath.Split(w.localPath)

	extractionDir := filepath.Join(baseDir, "extracted")

	mountDir := filepath.Join(baseDir, "mnt")
	if err := os.MkdirAll(mountDir, 0755); err != nil {
		return "", fmt.Errorf("could not create mount directory: %v", err)
	}

	_, err = shellCommand("sudo", "mount", w.localPath, mountDir)
	if err != nil {
		return "", fmt.Errorf("error while executing mount cmd: %v", err)
	}

	installWimPath := filepath.Join(mountDir, "/sources/install.wim")

	wimFile, err := os.Open(installWimPath)
	if err != nil {
		return "", fmt.Errorf("error while opening %s: %v", installWimPath, err)
	}

	reader, err := wim.NewReader(wimFile)
	if err != nil {
		return "", fmt.Errorf("error while creating wim reader %s: %v", installWimPath, err)
	}

	for _, image := range reader.Image {
		if image.Name == w.imageName {
			glog.Infof("Extracting files from %s located in %s to %s", image.Name, w.localPath, extractionDir)
			err := extractWimImage(image, extractionDir)
			if err != nil {
				return "", fmt.Errorf("error while extracting wim image %s: %v", image.Name, err)
			}
			glog.Infof("Done extracting files from %s", image.Name)
		}
	}

	time.Sleep(time.Second * 10)
	_, err = shellCommand("sudo", "umount", "-fl", mountDir)
	if err != nil {
		return "", fmt.Errorf("error while executing umount cmd: %v", err)
	}

	return extractionDir, nil
}
func extractWimImage(image *wim.Image, extractionDir string) error {
	rootDir, err := image.Open()
	if err != nil {
		return fmt.Errorf("error while opening wim file %s: %v", image.Name, err)
	}

	if err := extractWimFolder(rootDir, rootDir.Name, extractionDir); err != nil {
		return err
	}

	return nil
}

func extractWimFolder(wimFile *wim.File, path, extractionDir string) error {
	files, err := wimFile.Readdir()
	if err != nil {
		return fmt.Errorf("error while opening wim file %s: %v", wimFile.Name, err)
	}
	for _, file := range files {
		dstPath := filepath.Join(extractionDir, path, file.Name)
		if file.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				glog.Errorf("Could not create destination directory %s: %v", dstPath, err)
				continue
			}
			err = extractWimFolder(file, filepath.Join(path, file.Name), extractionDir)
			if err != nil {
				glog.Errorf("Failed to extract Wim folder %s: %v", file.Name, err)
				continue
			}
		} else {
			if err := copyFile(file, dstPath); err != nil {
				glog.Errorf("Could not copy to destination file %s: %v", dstPath, err)
				continue
			}
		}
	}

	return nil
}

func copyFile(file *wim.File, dstPath string) error {
	destFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("error while creating destination file: %v", err)
	}

	content, err := file.Open()
	if err != nil {
		return fmt.Errorf("error while opening wim %s file for reading: %v", file.Name, err)
	}

	_, err = io.Copy(destFile, content)
	if err != nil {
		return fmt.Errorf("error while copying destination file %s: %v", file.Name, err)
	}

	destFile.Close()
	content.Close()

	return nil
}

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

// ID returns non-unique Windows ISO file ID.
func (w *wimImage) ID() string {
	return w.id
}

// RepoName returns repository name.
func (w *wimImage) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (w *wimImage) RepoPath() string {
	return w.repoPath
}

// LocalPath returns local path to a Windows ISO file.
func (w *wimImage) LocalPath() string {
	return w.localPath
}

// RemotePath returns remote path to a Windows ISO file.
func (w *wimImage) RemotePath() string {
	return w.remotePath
}

// QuickSHA256Hash calculates sha256 hash of a Windows ISO file.
func (w *wimImage) QuickSHA256Hash() (string, error) {
	return w.quickHash, nil
}

// Description provides additional description for a Windows ISO file.
func (w *wimImage) Description() string {
	return ""
}

// NewRepo returns new instance of a Windows ISO repository.
func NewRepo(ctx context.Context, repositoryPath string) (*Repo, error) {
	return &Repo{path: repositoryPath}, nil
}

// Repo holds data related to a Windows repository.
type Repo struct {
	path      string
	files     []string
	wimImages []*wimImage
}

type wimImage struct {
	id         string
	imageName  string
	localPath  string
	remotePath string
	quickHash  string
	repoPath   string
}

// RepoName returns repository name.
func (r *Repo) RepoName() string {
	return RepoName
}

// RepoPath returns repository path.
func (r *Repo) RepoPath() string {
	return r.path
}

// DiscoverRepo traverses the repository and looks for .iso files.
func (r *Repo) DiscoverRepo() ([]hashr.Source, error) {

	if err := filepath.Walk(r.path, walk(&r.files)); err != nil {
		return nil, err
	}

	for _, filePath := range r.files {
		tempDir, err := common.LocalTempDir(strings.ReplaceAll(strings.TrimPrefix(filePath, r.path+string(os.PathSeparator)), string(os.PathSeparator), "-"))
		if err != nil {
			return nil, fmt.Errorf("error while creating temp dir: %v", err)
		}

		mountDir := filepath.Join(tempDir, "mnt")
		if err := os.MkdirAll(mountDir, 0755); err != nil {
			return nil, fmt.Errorf("could not create mount directory: %v", err)
		}

		_, err = shellCommand("sudo", "mount", filePath, mountDir)
		if err != nil {
			return nil, fmt.Errorf("error while executing mount cmd: %v", err)
		}

		installWimPath := filepath.Join(mountDir, "/sources/install.wim")

		wimFile, err := os.Open(installWimPath)
		if err != nil {
			return nil, fmt.Errorf("error while opening %s: %v", installWimPath, err)
		}

		reader, err := wim.NewReader(wimFile)
		if err != nil {
			return nil, fmt.Errorf("error while creating wim reader %s: %v", installWimPath, err)
		}

		glog.Infof("Opened %s wim file", installWimPath)

		for _, image := range reader.Image {
			glog.Infof("Found %s image in %s", image.Name, installWimPath)
			r.wimImages = append(r.wimImages, &wimImage{
				imageName:  image.Name,
				id:         fmt.Sprintf("%s-%d.%d-%d-%dsp", strings.ReplaceAll(image.Name, " ", ""), image.Windows.Version.Major, image.Windows.Version.Minor, image.Windows.Version.Build, image.Windows.Version.SPBuild),
				localPath:  filePath,
				remotePath: filePath,
				repoPath:   r.path,
				quickHash: fmt.Sprintf("%x", sha256.Sum256([]byte(image.CreationTime.Time().String()+
					image.Name+
					image.Windows.ProductName+
					strconv.Itoa(image.Windows.Version.Build)+
					strconv.Itoa(image.Windows.Version.Major)+
					strconv.Itoa(image.Windows.Version.Minor)+
					strconv.Itoa(image.Windows.Version.SPBuild)))),
			})
		}

		wimFile.Close()

		time.Sleep(time.Second * 10)
		_, err = shellCommand("sudo", "umount", "-fl", mountDir)
		if err != nil {
			return nil, fmt.Errorf("error while executing umount cmd: %v", err)
		}
	}

	var sources []hashr.Source
	for _, wimImage := range r.wimImages {
		sources = append(sources, wimImage)
	}

	return sources, nil
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

		if strings.EqualFold(filepath.Ext(info.Name()), ".iso") {
			*files = append(*files, path)
		}

		return nil
	}
}
