#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test ./internal/ai -run 'TestHybridEstimatorAndOfferUseRanges' -count=25
