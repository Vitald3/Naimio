#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test ./internal/acquisition -run 'TestDeterministicCalculatorCreatesPreservedDraftWithoutAI|TestSitemapIsBoundedToRealDefinitions' -count=25
