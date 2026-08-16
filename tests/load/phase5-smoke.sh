#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test -count=20 ./internal/growth -run '^TestInviteDirectionsSafePreviewAcceptanceAndRewardIdempotency$'
