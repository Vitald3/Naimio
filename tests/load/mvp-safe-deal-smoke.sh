#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test ./internal/safedeal -run 'TestSafeDealFundingRevisionReleaseEndToEnd|TestDisputeEvidenceAndAdminRefund' -count=25
