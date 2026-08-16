#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test -count=1 ./internal/reputation ./internal/reviews -run 'Test(ExternalReputationCRUDOwnershipAndState|VerificationChallengeAndModeratorDecision|PublicProjectionContainsOnlyVerifiedSafeFields|ReviewEligibilityDimensionsSelfAndDuplicate|TrustMetricsAndPublicPagination|ReviewReportAndModerationRecalculateTrust)'
