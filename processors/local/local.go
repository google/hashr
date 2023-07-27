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

// Package local provides functions to process data locally.
package local

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/golang/glog"
)

var execute = func(name string, args ...string) *exec.Cmd {
	glog.Infof("name: %v, args: %v", name, args)
	return exec.Command(name, args...)
}

// Processor is an instance of local processor.
type Processor struct {
}

// New returns new local processor instance.
func New() *Processor {
	return &Processor{}
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

// ImageExport runs image_export.py binary locally.
func (p *Processor) ImageExport(sourcePath string) (string, error) {
	// TODO(mlegin): check if image_export.py is present on the local machine.
	baseDir := filepath.Dir(sourcePath)
	exportDir := filepath.Join(baseDir, "export")
	logFile := filepath.Join(baseDir, "image_export.log")

	dockerArgs := []string{"run", "--rm", "-v", "/tmp/:/tmp", "log2timeline/plaso", "image_export", "--logfile", logFile, "--partitions", "all", "--volumes", "all", "-w", exportDir, sourcePath}
	localArgs := []string{"--logfile", logFile, "--partitions", "all", "--volumes", "all", "-w", exportDir, sourcePath}
	var err error

	if inDockerContainer() {
		_, err = shellCommand("image_export.py", localArgs...)
	} else {
		_, err = shellCommand("docker", dockerArgs...)
	}

	if err != nil {
		return "", fmt.Errorf("error while running image_export: %v", err)
	}

	return exportDir, nil
}

func inDockerContainer() bool {
	_, err := shellCommand("ls", "/.dockerenv")
	return err == nil
}
