package supportnotify

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
)

const SupportUserID = "00000000-0000-4000-8000-000000000011"

var tagPattern = regexp.MustCompile(`<[^>]+>`)

func plain(value string) string {
	value = tagPattern.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}

// ModerationNotice writes both an in-app notification and a real support
// conversation message inside the caller's transaction. Every moderation decision
// gets its own notification/email event; only the support conversation is reused.
func ModerationNotice(ctx context.Context, tx *sql.Tx, userID, entityType, entityID, title, action, reason string) error {
	if tx == nil || userID == "" {
		return nil
	}
	editURL := ""
	switch strings.ToUpper(entityType) {
	case "PROJECT":
		editURL = "/dashboard/projects/" + entityID + "?edit=1"
	case "SERVICE":
		editURL = "/dashboard/services/" + entityID + "/edit"
	case "VACANCY":
		editURL = "/dashboard/vacancies/" + entityID + "/edit"
	case "REVIEW":
		editURL = "/dashboard/reviews"
	}
	payload, err := json.Marshal(map[string]any{"title": title, "action": action, "reason_html": reason, "reason_text": plain(reason), "edit_url": editURL})
	if err != nil {
		return err
	}
	// Each moderation decision is a distinct user-visible event. Do not dedupe
	// a later reject/delete against an older decision for the same entity.
	key := "moderation:" + strings.ToLower(entityType) + ":" + entityID + ":" + strings.ToLower(action) + ":" + userID + ":" + fmt.Sprint(time.Now().UTC().UnixNano())
	if len(key) > 200 {
		key = key[:200]
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO notifications(id,dedupe_key,user_id,type,actor_user_id,entity_type,entity_id,payload)
VALUES(gen_random_uuid(),$1,$2,'moderation_update',$3,$4,$5::uuid,$6::jsonb) ON CONFLICT(dedupe_key) DO NOTHING`, key, userID, SupportUserID, entityType, entityID, string(payload)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO email_jobs(dedupe_key,user_id,template,payload)
VALUES($1,$2,'moderation_update',$3::jsonb) ON CONFLICT(dedupe_key) DO NOTHING`, "email:"+key, userID, string(payload)); err != nil {
		return err
	}
	var conversationID string
	err = tx.QueryRowContext(ctx, `SELECT c.id::text FROM conversations c
JOIN conversation_members a ON a.conversation_id=c.id AND a.user_id=$1
JOIN conversation_members b ON b.conversation_id=c.id AND b.user_id=$2
WHERE c.kind='DIRECT' AND (SELECT count(*) FROM conversation_members x WHERE x.conversation_id=c.id)=2
ORDER BY c.created_at LIMIT 1`, SupportUserID, userID).Scan(&conversationID)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx, `INSERT INTO conversations(kind)VALUES('DIRECT') RETURNING id::text`).Scan(&conversationID)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id,user_id)VALUES($1,$2),($1,$3)`, conversationID, SupportUserID, userID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	message := "Поддержка Naimio: " + title + ". " + action + "."
	if r := plain(reason); r != "" {
		message += " Причина: " + r
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO messages(id,conversation_id,sender_user_id,type,body)VALUES(gen_random_uuid(),$1,$2,'SYSTEM',$3)`, conversationID, SupportUserID, message); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, conversationID)
	return err
}
