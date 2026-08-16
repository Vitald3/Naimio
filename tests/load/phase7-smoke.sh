#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test ./internal/jobs ./internal/services -run 'TestVacancyLifecycleSearchAndApplications|TestServiceTransitionsVisibilityFiltersAndPagination' -count=25
