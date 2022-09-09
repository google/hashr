-- Copyright 2022 Google LLC
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--      https:--www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.

CREATE TABLE samples (
        sha256 STRING(100),
        mimetype STRING(MAX),
        file_output  STRING(MAX),
        size INT64
) PRIMARY KEY(sha256);

CREATE TABLE payloads (
        sha256 STRING(100),
        payload BYTES(MAX)
) PRIMARY KEY(sha256);

CREATE TABLE sources (
        sha256 STRING(100),
        sourceID  ARRAY<STRING(MAX)>,
        sourcePath  STRING(MAX),
        sourceDescription STRING(MAX),
        repoName STRING(MAX),
        repoPath STRING(MAX),
) PRIMARY KEY(sha256);

CREATE TABLE samples_sources (
        sample_sha256 STRING(100),
        source_sha256 STRING(100),
        sample_paths ARRAY<STRING(MAX)>,
        CONSTRAINT FK_Sample FOREIGN KEY (sample_sha256) REFERENCES samples (sha256),
        CONSTRAINT FK_Source FOREIGN KEY (source_sha256) REFERENCES sources (sha256),
)  PRIMARY KEY (sample_sha256, source_sha256);