#!/bin/sh
set -eu
cd "$(dirname "$0")/../../apps/api"
go test -count=1 ./cmd/api ./internal/communication ./internal/notifications -run 'Test(Phase4RoutesUseSessionAndNamedChatRateLimit|ConversationAuthorizationIdempotencyAndReadState|ChatAttachmentMustBeOwnedCleanAndChatPurpose|HTTPRejectsUnauthenticatedAndUnknownFields|HubPublishesOnlyToNamedUsers|RealtimeRequiresAuthentication|NotificationOwnershipReadAndPreferences|NotificationHandlerAuthenticationAndStrictInput)'
