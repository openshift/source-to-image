#
# Copyright 2023 Go Imports Organizer Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# 	https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
.DEFAULT_GOAL := help

verify: verify-imports verify-gofmt ## Run verifications. Example: make verify
.PHONY: verify

verify-imports: ## Run import verifications. Example: make verify-imports
	scripts/verify-imports.sh
.PHONY: verify

verify-gofmt: ## Run gofmt verifications. Example: make verify-gofmt
	scripts/verify-gofmt.sh
.PHONY: verify

imports: ## Organize imports in go files using goio. Example: make imports
	go run main.go
.PHONY: imports

test: ## Run tests. Example: make test
	go test -test.shuffle on ./pkg/... --cover
.PHONY: test

build: ## Build the executable. Example: make build
	@go version
	go build -mod=readonly -race $(DEBUGFLAGS)
.PHONY: build

clean: ## Clean up the workspace. Example: make clean
	rm -rf goio *.pprof profile*
.PHONY: clean

help: ## Print this help. Example: make help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
.PHONY: help
