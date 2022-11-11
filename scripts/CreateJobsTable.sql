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

CREATE TABLE jobs (
         quick_sha256 VARCHAR(100) PRIMARY KEY,
          imported_at INT NOT NULL,
          id text,
          repo text,
          repo_path text,
          location text,
          sha256 VARCHAR(100),
          status VARCHAR(50),
          error text,
          preprocessing_duration INT,
          processing_duration INT,
          export_duration INT,
          files_extracted INT,
          files_exported INT
);