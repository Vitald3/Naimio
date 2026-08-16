#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test ./internal/ai ./internal/matching -run 'TestHybridEstimator|TestDeterministicSignals' -count=25
