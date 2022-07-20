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

package gcp

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	storage "google.golang.org/api/storage/v1"
)

type mockTransport struct {
	responses []mockResponses
	index     int
}

// RoundTrip is an http.Client.Transport implementation. This has to be exported to satisfy
// http.RoundTripper interface.
func (c *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	if c.index >= len(c.responses) {
		resp = &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			StatusCode: 503,
			Status: fmt.Sprintf("Got more requests than mocked Response. Index: %d, Response: %d",
				c.index, len(c.responses)),
			Header:  make(http.Header),
			Request: req,
		}
	} else {
		header := make(http.Header)
		for k, v := range c.responses[c.index].Header {
			header.Add(k, v)
		}
		body := ioutil.NopCloser(strings.NewReader(c.responses[c.index].Body))
		resp = &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			StatusCode: c.responses[c.index].StatusCode,
			Body:       body,
			Header:     header,
			Request:    req,
		}
	}
	c.index++
	return resp, nil
}

func (c *mockTransport) add(StatusCode int, Body string, Header map[string]string) {
	c.responses = append(c.responses, mockResponses{StatusCode, Header, Body})
}

type mockResponses struct {
	StatusCode int
	Header     map[string]string
	Body       string
}

func mockHTTPClientAndTransport() (*http.Client, *mockTransport) {
	mockTransport := mockTransport{make([]mockResponses, 0), 0}
	httpClient := &http.Client{}
	httpClient.Transport = &mockTransport
	return httpClient, &mockTransport
}

var (
	imagesListResponse = `{
  "id": "projects/gce-test-images/global/images",
  "items": [
    {
      "archiveSizeBytes": "840734400",
      "creationTimestamp": "2020-04-24T01:38:09.958-07:00",
      "diskSizeGb": "10",
      "id": "553253379704298398",
      "kind": "compute#image",
      "name": "ubuntu-1804-lts-drawfork-v20190613"
    },
		{
      "archiveSizeBytes": "850982200",
      "creationTimestamp": "2020-04-25T03:36:09.934-07:00",
      "diskSizeGb": "10",
      "id": "553253379704294412",
      "kind": "compute#image",
      "name": "ubuntu-1804-lts-drawfork-v20190616"
    },
    {
      "archiveSizeBytes": "840734400",
      "creationTimestamp": "2020-04-24T01:38:09.958-07:00",
      "deprecated": {
        "replacement": "https://www.googleapis.com/compute/v1/projects/ubuntu-os-cloud/global/images/ubuntu-1204-precise-v20170502",
        "state": "DEPRECATED"
      },
      "diskSizeGb": "10",
      "id": "553253379704298398",
      "kind": "compute#image",
      "name": "ubuntu-1804-lts-drawfork-v20190613"
    }
  ],
  "kind": "compute#imageList",
  "selfLink": "https://www.googleapis.com/compute/v1/projects/gce-test-images/global/images"
}`
	image1GetResponse = `{
  "archiveSizeBytes": "840734400",
  "creationTimestamp": "2020-04-24T01:38:09.958-07:00",
  "id": "553253379704298398",
  "name": "ubuntu-1804-lts-drawfork-v20190613",
	"diskSizeGb": "10"
}`
	image2GetResponse = `{
  "archiveSizeBytes": "8409871200",
  "creationTimestamp": "2020-04-25T01:22:09.312-07:00",
  "id": "553253379704292343",
  "name": "ubuntu-1804-lts-drawfork-v20190618",
	"diskSizeGb": "10"
}`
	imageInsertResponse = `{
  "id": "2422925859985654420",
  "insertTime": "2020-07-28T07:55:55.337-07:00",
  "kind": "compute#operation",
  "name": "operation-1595948154612-5ab81a2d5eb1b-75a13304-e3561fdd",
  "operationType": "insert",
  "selfLink": "https://www.googleapis.com/compute/v1/projects/hashr/global/operations/operation-1595948154612-5ab81a2d5eb1b-75a13304-e3561fdd",
  "startTime": "2020-07-28T07:55:55.342-07:00",
  "status": "RUNNING",
  "targetId": "6974136402423153301",
  "targetLink": "https://www.googleapis.com/compute/v1/projects/hashr/global/images/ubuntu-1804-lts-drawfork-v20190613",
  "user": "mlegin@google.com"
}`
	globalOperationsResponse = `{
  "endTime": "2020-07-28T07:56:50.859-07:00",
  "id": "2422925859985654420",
  "insertTime": "2020-07-28T07:55:55.337-07:00",
  "kind": "compute#operation",
  "name": "operation-1595948154612-5ab81a2d5eb1b-75a13304-e3561fdd",
  "operationType": "insert",
  "progress": 100,
  "selfLink": "https://www.googleapis.com/compute/v1/projects/hashr/global/operations/operation-1595948154612-5ab81a2d5eb1b-75a13304-e3561fdd",
  "startTime": "2020-07-28T07:55:55.342-07:00",
  "status": "DONE",
  "targetId": "6974136402423153301",
  "targetLink": "https://www.googleapis.com/compute/v1/projects/hashr/global/images/ubuntu-1804-lts-drawfork-v20190613",
  "user": "mlegin@google.com"
}`
	cloudBuildCreateResponse = `{
  "metadata": {
    "@type": "type.googleapis.com/google.devtools.cloudbuild.v1.BuildOperationMetadata",
    "build": {
      "id": "215d8151-5ce2-4dd5-8afe-0838fbe28419",
      "status": "QUEUED",
      "createTime": "2020-07-28T14:56:57.642861345Z",
      "steps": [
        {
          "name": "gcr.io/compute-image-tools/gce_vm_image_export:release",
          "env": [
            "BUILD_ID=215d8151-5ce2-4dd5-8afe-0838fbe28419"
          ],
          "args": [
            "-client_id=api",
            "-timeout=1800s",
            "-source_image=ubuntu-1804-lts-drawfork-v20190613",
            "-destination_uri=gs://hashr-images/hashr-dev-ubuntu-1804-lts-drawfork-v20190613.tar.gz"
          ]
        }
      ],
      "timeout": "1800s",
      "projectId": "hashr",
      "logsBucket": "gs://1081703565750.cloudbuild-logs.googleusercontent.com",
      "options": {
        "logging": "LEGACY"
      },
      "logUrl": "https://console.cloud.google.com/cloud-build/builds/215d8151-5ce2-4dd5-8afe-0838fbe28419?project=1081703565750",
      "tags": [
        "gce-daisy",
        "gce-daisy-image-export"
      ],
      "queueTtl": "3600s"
    }
  },
  "name": "operations/build/hashr/MjE1ZDgxNTEtNWNlMi00ZGQ1LThhZmUtMDgzOGZiZTI4NDE5"
}`
	cloudBuildGetInProgressResponse = `{
  "done": true,
  "metadata": {
    "@type": "type.googleapis.com/google.devtools.cloudbuild.v1.BuildOperationMetadata",
    "build": {
      "id": "215d8151-5ce2-4dd5-8afe-0838fbe28419",
      "status": "SUCCESS",
      "createTime": "2020-07-28T14:56:57.642861345Z",
      "startTime": "2020-07-28T14:56:58.516373155Z",
      "finishTime": "2020-07-28T14:59:44.530954Z",
      "results": {
        "buildStepImages": [
          ""
        ],
        "buildStepOutputs": [
          ""
        ]
      },
      "steps": [
        {
          "name": "gcr.io/compute-image-tools/gce_vm_image_export:release",
          "env": [
            "BUILD_ID=215d8151-5ce2-4dd5-8afe-0838fbe28419"
          ],
          "args": [
            "-client_id=api",
            "-timeout=1800s",
            "-source_image=ubuntu-1804-lts-drawfork-v20190613",
            "-destination_uri=gs://hashr-images/hashr-dev-ubuntu-1804-lts-drawfork-v20190613.tar.gz"
          ],
          "timing": {
            "startTime": "2020-07-28T14:57:04.700015129Z",
            "endTime": "2020-07-28T14:59:43.708501306Z"
          },
          "status": "SUCCESS",
          "pullTiming": {
            "startTime": "2020-07-28T14:57:04.700015129Z",
            "endTime": "2020-07-28T14:57:08.494431605Z"
          }
        }
      ],
      "timeout": "1800s",
      "projectId": "hashr",
      "logsBucket": "gs://1081703565750.cloudbuild-logs.googleusercontent.com",
      "sourceProvenance": {},
      "options": {
        "logging": "LEGACY"
      },
      "logUrl": "https://console.cloud.google.com/cloud-build/builds/215d8151-5ce2-4dd5-8afe-0838fbe28419?project=1081703565750",
      "tags": [
        "gce-daisy",
        "gce-daisy-image-export"
      ],
      "timing": {
        "BUILD": {
          "startTime": "2020-07-28T14:57:03.398184413Z",
          "endTime": "2020-07-28T14:59:43.708575737Z"
        }
      },
      "queueTtl": "3600s"
    }
  },
  "name": "operations/build/hashr/MjE1ZDgxNTEtNWNlMi00ZGQ1LThhZmUtMDgzOGZiZTI4NDE5",
  "response": {
    "@type": "type.googleapis.com/google.devtools.cloudbuild.v1.Build",
    "id": "215d8151-5ce2-4dd5-8afe-0838fbe28419",
    "status": "SUCCESS",
    "createTime": "2020-07-28T14:56:57.642861345Z",
    "startTime": "2020-07-28T14:56:58.516373155Z",
    "finishTime": "2020-07-28T14:59:44.530954Z",
    "results": {
      "buildStepImages": [
        ""
      ],
      "buildStepOutputs": [
        ""
      ]
    },
    "steps": [
      {
        "name": "gcr.io/compute-image-tools/gce_vm_image_export:release",
        "env": [
          "BUILD_ID=215d8151-5ce2-4dd5-8afe-0838fbe28419"
        ],
        "args": [
          "-client_id=api",
          "-timeout=1800s",
          "-source_image=ubuntu-1804-lts-drawfork-v20190613",
          "-destination_uri=gs://hashr-images/hashr-dev-ubuntu-1804-lts-drawfork-v20190613.tar.gz"
        ],
        "timing": {
          "startTime": "2020-07-28T14:57:04.700015129Z",
          "endTime": "2020-07-28T14:59:43.708501306Z"
        },
        "status": "SUCCESS",
        "pullTiming": {
          "startTime": "2020-07-28T14:57:04.700015129Z",
          "endTime": "2020-07-28T14:57:08.494431605Z"
        }
      }
    ],
    "timeout": "1800s",
    "projectId": "hashr",
    "logsBucket": "gs://1081703565750.cloudbuild-logs.googleusercontent.com",
    "sourceProvenance": {},
    "options": {
      "logging": "LEGACY"
    },
    "logUrl": "https://console.cloud.google.com/cloud-build/builds/215d8151-5ce2-4dd5-8afe-0838fbe28419?project=1081703565750",
    "tags": [
      "gce-daisy",
      "gce-daisy-image-export"
    ],
    "timing": {
      "BUILD": {
        "startTime": "2020-07-28T14:57:03.398184413Z",
        "endTime": "2020-07-28T14:59:43.708575737Z"
      }
    },
    "queueTtl": "3600s"
  }
}`
	cloudBuildGetDoneResponse = `{
  "createTime": "2020-07-28T14:56:57.642861345Z",
  "finishTime": "2020-07-28T14:59:44.530954Z",
  "id": "215d8151-5ce2-4dd5-8afe-0838fbe28419",
  "logUrl": "https://console.cloud.google.com/cloud-build/builds/215d8151-5ce2-4dd5-8afe-0838fbe28419?project=1081703565750",
  "logsBucket": "gs://1081703565750.cloudbuild-logs.googleusercontent.com",
  "options": {
    "logging": "LEGACY"
  },
  "projectId": "hashr",
  "queueTtl": "3600s",
  "results": {
    "buildStepImages": [
      ""
    ],
    "buildStepOutputs": [
      ""
    ]
  },
  "sourceProvenance": {},
  "startTime": "2020-07-28T14:56:58.516373155Z",
  "status": "SUCCESS",
  "steps": [
    {
      "args": [
        "-client_id=api",
        "-timeout=1800s",
        "-source_image=ubuntu-1804-lts-drawfork-v20190613",
        "-destination_uri=gs://hashr-images/hashr-dev-ubuntu-1804-lts-drawfork-v20190613.tar.gz"
      ],
      "env": [
        "BUILD_ID=215d8151-5ce2-4dd5-8afe-0838fbe28419"
      ],
      "name": "gcr.io/compute-image-tools/gce_vm_image_export:release",
      "pullTiming": {
        "endTime": "2020-07-28T14:57:08.494431605Z",
        "startTime": "2020-07-28T14:57:04.700015129Z"
      },
      "status": "SUCCESS",
      "timing": {
        "endTime": "2020-07-28T14:59:43.708501306Z",
        "startTime": "2020-07-28T14:57:04.700015129Z"
      }
    }
  ],
  "tags": [
    "gce-daisy",
    "gce-daisy-image-export"
  ],
  "timeout": "1800s",
  "timing": {
    "BUILD": {
      "endTime": "2020-07-28T14:59:43.708575737Z",
      "startTime": "2020-07-28T14:57:03.398184413Z"
    }
  }
}`
)

func TestDiscoverRepo(t *testing.T) {
	ctx := context.Background()
	repo := &Repo{projectName: "gce-test-images"}
	client, transport := mockHTTPClientAndTransport()
	transport.add(200, imagesListResponse, nil)

	var err error
	computeClient, err = compute.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		t.Fatalf("could not create mock GCE client: %v", err)
	}

	gotSources, err := repo.DiscoverRepo()
	if err != nil {
		t.Fatalf("unexpected error while running DiscoverRepo(): %v", err)
	}

	wantImages := []*Image{
		{
			id:      "gce-test-images-ubuntu-1804-lts-drawfork-v20190613",
			name:    "ubuntu-1804-lts-drawfork-v20190613",
			project: "gce-test-images",
		},
		{
			id:      "gce-test-images-ubuntu-1804-lts-drawfork-v20190616",
			name:    "ubuntu-1804-lts-drawfork-v20190616",
			project: "gce-test-images",
		},
	}

	var gotImages []*Image
	for _, source := range gotSources {
		if image, ok := source.(*Image); ok {
			gotImages = append(gotImages, image)
		} else {
			t.Fatal("error while casting Source interface to Image struct")
		}
	}

	if !cmp.Equal(wantImages, gotImages, cmp.AllowUnexported(Image{})) {
		t.Errorf("DiscoverRepo() unexpected diff (-want/+got):\n%s", cmp.Diff(wantImages, gotImages, cmp.AllowUnexported(Image{})))
	}
}

func TestQuickSHA256Hash(t *testing.T) {
	ctx := context.Background()
	client, transport := mockHTTPClientAndTransport()
	transport.add(200, image1GetResponse, nil)
	transport.add(200, image2GetResponse, nil)

	var err error
	computeClient, err = compute.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		t.Fatalf("could not create mock GCE client: %v", err)
	}

	for _, tc := range []struct {
		image    *Image
		wantHash string
	}{
		{
			image:    &Image{name: "ubuntu-1804-lts-drawfork-v20190613", project: "gce-test-images", quickSha256hash: "4e2998b54e3216894520bb9fc84255f9ec3f38b4183599c22bd5d6f7f5fe2bd9"},
			wantHash: "4e2998b54e3216894520bb9fc84255f9ec3f38b4183599c22bd5d6f7f5fe2bd9",
		},
		{
			image:    &Image{name: "ubuntu-1804-lts-drawfork-v20190613", project: "gce-test-images"},
			wantHash: "fa52543a57f7f5fd7b064ec7d5fb412cda6d5fb7e334138d3490743425f42815",
		},
		{
			image:    &Image{name: "ubuntu-1804-lts-drawfork-v20190618", project: "gce-test-images"},
			wantHash: "dfa6b612b0907c2814355aafa34be6e0da8e25bc3b51a1c22947ca5e1732e60a",
		},
	} {
		gotHash, err := tc.image.QuickSHA256Hash()
		if err != nil {
			t.Fatalf("Unexpected error while running QuickSHA256Hash(): %v", err)
		}

		if gotHash != tc.wantHash {
			t.Errorf("QuickSHA256Hash() = %v; want = %v", gotHash, tc.wantHash)
		}
	}
}

func TestPreprocess(t *testing.T) {
	ctx := context.Background()
	computeHTTPClient, computeHTTPTransport := mockHTTPClientAndTransport()
	computeHTTPTransport.add(200, imageInsertResponse, nil)
	computeHTTPTransport.add(200, imageInsertResponse, nil)
	computeHTTPTransport.add(200, globalOperationsResponse, nil)

	var err error
	computeClient, err = compute.NewService(ctx, option.WithHTTPClient(computeHTTPClient))
	if err != nil {
		t.Fatalf("could not create mock GCE client: %v", err)
	}

	cloudBuildHTTPClient, cloudBuildHTTPTransport := mockHTTPClientAndTransport()
	cloudBuildHTTPTransport.add(200, cloudBuildCreateResponse, nil)
	cloudBuildHTTPTransport.add(200, cloudBuildGetInProgressResponse, nil)
	cloudBuildHTTPTransport.add(200, cloudBuildGetDoneResponse, nil)

	cloudBuildClient, err = cloudbuild.NewService(ctx, option.WithHTTPClient(cloudBuildHTTPClient))
	if err != nil {
		t.Fatalf("could not create mock Cloud Build client: %v", err)
	}

	bytes, err := ioutil.ReadFile("testdata/ubuntu-1804-lts-drawfork-v20190613.tar.gz")
	if err != nil {
		t.Fatalf("could not open test GCP file: %v", err)
	}

	storageHTTPClient, storageHTTPTransport := mockHTTPClientAndTransport()
	storageHTTPTransport.add(200, string(bytes), nil)

	storageClient, err = storage.NewService(ctx, option.WithHTTPClient(storageHTTPClient))
	if err != nil {
		t.Fatalf("could not create mock GCE client: %v", err)
	}

	testImage := &Image{
		project: "hashr-dev",
		name:    "ubuntu-1804-lts-drawfork-v20190613",
		id:      "hashr-dev-ubuntu-1804-lts-drawfork-v20190613",
	}

	path, err := testImage.Preprocess()
	if err != nil {
		t.Fatalf("Unexpected error while running Preprocess(): %v", err)
	}

	bytes, err = ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("could not open extracted disk.raw file: %v", err)
	}

	gotHash := fmt.Sprintf("%x", sha256.Sum256(bytes))
	wantHash := "12ccd8dfadb99d2bd0ecdc4ebeee4ad8b0efa10d7a47bf312c12da572a237fd4"

	if gotHash != wantHash {
		t.Errorf("Preprocess() = %v; want = %v", gotHash, wantHash)
	}
}

func TestNewRepo(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		gcpProject   string
		wantRepoName string
		wantRepoPath string
	}{
		{
			gcpProject:   "hashr-dev",
			wantRepoName: "GCP",
			wantRepoPath: "hashr-dev",
		},
		{
			gcpProject:   "eip-images",
			wantRepoName: "GCP",
			wantRepoPath: "eip-images",
		},
	} {
		repo, err := NewRepo(ctx, nil, nil, nil, tc.gcpProject, "", "")
		if err != nil {
			t.Fatalf("Unexpected error while running NewRepo(): %v", err)
		}

		if repo.RepoName() != tc.wantRepoName {
			t.Errorf("Unexpected value returned by RepoName() = %s, want = %s", repo.RepoName(), tc.wantRepoName)
		}
		if repo.RepoPath() != tc.wantRepoPath {
			t.Errorf("Unexpected value returned by RepoPath() = %s, want = %s", repo.RepoPath(), tc.wantRepoPath)
		}
	}
}
