// Copyright 2023 Google LLC
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

// Package aws implements AWS repository importer unit tests.

package aws

import (
	"bytes"
	"context"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type mockDescribeImagesAPI func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)

func (m mockDescribeImagesAPI) DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return m(ctx, params, optFns...)
}

func TestDiscoveryRepo(t *testing.T) {
	cases := []struct {
		client       func(t *testing.T) ec2DescribeImagesAPI
		architecture []string
		expect       []byte
	}{
		{
			client: func(t *testing.T) ec2DescribeImagesAPI {
				return mockDescribeImagesAPI(func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
					t.Helper()

					return &ec2.DescribeImagesOutput{
						Images: []types.Image{
							{
								ImageId: aws.String("ami-sample"),
							},
						},
					}, nil
				})
			},
			architecture: []string{"x86_64"},
			expect:       []byte("ami-sample"),
		},
	}

	for i, tt := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			ctx := context.TODO()
			images, err := getAmazonImages(ctx, tt.client(t), tt.architecture)
			if err != nil {
				t.Fatalf("expect no error, got %v", err)
			}
			if e, a := tt.expect, []byte(*images[0].ImageId); bytes.Compare(e, a) != 0 {
				t.Errorf("expect %v, got %v", e, a)
			}
		})
	}
}
