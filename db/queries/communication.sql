-- name: ListConversations :many
SELECT c.id,c.kind,c.project_id,c.updated_at FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id WHERE cm.user_id=$1 AND cm.archived_at IS NULL ORDER BY c.updated_at DESC,c.id DESC;
-- name: ListMessages :many
SELECT id,conversation_id,sender_user_id,type,body,reply_to_message_id,client_message_id,edited_at,deleted_at,created_at FROM messages WHERE conversation_id=$1 AND($2::timestamptz IS NULL OR(created_at,id)<($2,$3))ORDER BY created_at DESC,id DESC LIMIT $4;
-- name: ListNotifications :many
SELECT id,type,actor_user_id,entity_type,entity_id,payload,read_at,created_at FROM notifications WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2;
