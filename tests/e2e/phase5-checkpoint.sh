#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test -count=1 ./internal/auth ./internal/growth ./internal/proposals ./cmd/api -run 'Test(Argon2PasswordHashAndSecureSessionCookie|RegisterValidationAndCSRF|InviteDirectionsSafePreviewAcceptanceAndRewardIdempotency|ProjectInviteRepeatShareAndTeamAuthorization|GrowthHTTPUsesAuthenticatedActorAndStrictPayload|Phase5RoutesUseSharedSecurityControls)'
