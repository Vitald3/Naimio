#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test -count=1 ./cmd/api -run '^TestPhase2PublicReadLoadSmoke$'
