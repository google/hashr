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

package local

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExecute(t *testing.T) {
	bytes, err := execute("echo", "test").Output()
	if err != nil {
		t.Fatalf("unexpected error while running test echo cmd: %v", err)
	}
	if got, want := string(bytes), "test\n"; got != want {
		t.Errorf("echo = %s; want = %s", got, want)
	}
}

func TestImageExport(t *testing.T) {
	execute = fakeExecute
	tempDir, err := ioutil.TempDir("", "hashr-test")
	if err != nil {
		t.Fatalf("error while creating temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sourceFile, err := os.Open("testdata/disk_2_xfs_volumes.raw")
	if err != nil {
		t.Fatalf("unexpected error while opening test WIM file: %v", err)
	}

	xfsTempPath := filepath.Join(tempDir, "disk_2_xfs_volumes.raw")
	destFile, err := os.Create(xfsTempPath)
	if err != nil {
		t.Fatalf("unexpected error creating temp destination file: %v", err)
	}

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		t.Fatalf("unexpected error while copying to temp destination file: %v", err)
	}

	processor := New()
	gotOut, err := processor.ImageExport(xfsTempPath)
	if err != nil {
		t.Fatalf("unexpected error while running ImageExport(): %v", err)
	}

	wantOut := filepath.Join(tempDir, "export")

	if gotOut != wantOut {
		t.Errorf("ImageExport() = %s; want = %s", gotOut, wantOut)
	}

}

func fakeExecute(command string, args ...string) *exec.Cmd {
	var mockStdOut string

	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1",
		"STDOUT=" + mockStdOut}
	return cmd
}

// This isn't a real test. It's used as a helper process.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Fprint(os.Stdout, os.Getenv("STDOUT"))
	os.Exit(0)
}
