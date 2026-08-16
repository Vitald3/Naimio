package communication

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) CreateConversation(ctx context.Context, a string, in CreateConversation) (Conversation, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Conversation{}, e
	}
	defer tx.Rollback()
	kind := "DIRECT"
	var project any
	members := []string{a}
	if in.ProjectID != "" {
		kind = "PROJECT"
		project = in.ProjectID
		rows, e := tx.QueryContext(ctx, `SELECT user_id::text FROM(SELECT customer_user_id user_id FROM projects WHERE id=$1 UNION SELECT freelancer_user_id FROM project_assignments WHERE project_id=$1 AND status IN('ACTIVE','COMPLETED'))x`, in.ProjectID)
		if e != nil {
			return Conversation{}, e
		}
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			if id != a {
				members = append(members, id)
			}
		}
		rows.Close()
		if len(members) < 2 {
			return Conversation{}, ErrNotFound
		}
		var id string
		e = tx.QueryRowContext(ctx, `SELECT id::text FROM conversations WHERE project_id=$1`, in.ProjectID).Scan(&id)
		if e == nil {
			_ = tx.Rollback()
			return r.GetConversation(ctx, a, id)
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return Conversation{}, e
		}
	} else {
		var active bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND status='ACTIVE' AND deleted_at IS NULL)`, in.ParticipantUserID).Scan(&active); e != nil {
			return Conversation{}, e
		}
		if !active {
			return Conversation{}, ErrNotFound
		}
		members = append(members, in.ParticipantUserID)
		var id string
		e = tx.QueryRowContext(ctx, `SELECT c.id::text FROM conversations c WHERE c.kind='DIRECT' AND(SELECT array_agg(user_id ORDER BY user_id)FROM conversation_members WHERE conversation_id=c.id)=ARRAY[$1::uuid,$2::uuid]`, a, in.ParticipantUserID).Scan(&id)
		if e == nil {
			_ = tx.Rollback()
			return r.GetConversation(ctx, a, id)
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return Conversation{}, e
		}
	}
	var id string
	e = tx.QueryRowContext(ctx, `INSERT INTO conversations(kind,project_id)VALUES($1,$2)RETURNING id::text`, kind, project).Scan(&id)
	if e != nil {
		return Conversation{}, mapDB(e)
	}
	for _, m := range members {
		if _, e = tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id,user_id)VALUES($1,$2)`, id, m); e != nil {
			return Conversation{}, e
		}
	}
	if e = tx.Commit(); e != nil {
		return Conversation{}, e
	}
	return r.GetConversation(ctx, a, id)
}
func (r PostgresRepository) ListConversations(ctx context.Context, a string) ([]Conversation, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT c.id::text,c.kind,c.project_id::text,c.created_at,c.updated_at,(SELECT count(*) FROM messages m WHERE m.conversation_id=c.id AND m.sender_user_id<>$1 AND(cm.last_read_message_id IS NULL OR EXISTS(SELECT 1 FROM messages previous WHERE previous.id=cm.last_read_message_id AND(m.created_at,m.id)>(previous.created_at,previous.id)))),COALESCE(p.title,''),COALESCE((SELECT u.display_name FROM conversation_members other JOIN users u ON u.id=other.user_id WHERE other.conversation_id=c.id AND other.user_id<>$1 ORDER BY other.joined_at LIMIT 1),''),COALESCE((SELECT other.user_id::text FROM conversation_members other WHERE other.conversation_id=c.id AND other.user_id<>$1 ORDER BY other.joined_at LIMIT 1),''),COALESCE((SELECT u.username FROM conversation_members other JOIN users u ON u.id=other.user_id WHERE other.conversation_id=c.id AND other.user_id<>$1 ORDER BY other.joined_at LIMIT 1),''),CASE WHEN EXISTS(SELECT 1 FROM conversation_members other JOIN user_capabilities uc ON uc.user_id=other.user_id AND uc.capability='FREELANCER' WHERE other.conversation_id=c.id AND other.user_id<>$1) THEN 'FREELANCER' ELSE 'CUSTOMER' END FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id LEFT JOIN projects p ON p.id=c.project_id WHERE cm.user_id=$1 AND cm.archived_at IS NULL ORDER BY c.updated_at DESC,c.id DESC`, a)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var v Conversation
		var p sql.NullString
		if e = rows.Scan(&v.ID, &v.Kind, &p, &v.CreatedAt, &v.UpdatedAt, &v.UnreadCount, &v.ProjectTitle, &v.CounterpartyName, &v.CounterpartyID, &v.CounterpartyUsername, &v.CounterpartyRole); e != nil {
			return nil, e
		}
		if p.Valid {
			v.ProjectID = &p.String
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r PostgresRepository) GetConversation(ctx context.Context, a, id string) (Conversation, error) {
	var v Conversation
	var p sql.NullString
	e := r.DB.QueryRowContext(ctx, `SELECT c.id::text,c.kind,c.project_id::text,c.created_at,c.updated_at,COALESCE(p.title,''),COALESCE((SELECT u.display_name FROM conversation_members other JOIN users u ON u.id=other.user_id WHERE other.conversation_id=c.id AND other.user_id<>$2 ORDER BY other.joined_at LIMIT 1),''),COALESCE((SELECT other.user_id::text FROM conversation_members other WHERE other.conversation_id=c.id AND other.user_id<>$2 ORDER BY other.joined_at LIMIT 1),''),COALESCE((SELECT u.username FROM conversation_members other JOIN users u ON u.id=other.user_id WHERE other.conversation_id=c.id AND other.user_id<>$2 ORDER BY other.joined_at LIMIT 1),''),CASE WHEN EXISTS(SELECT 1 FROM conversation_members other JOIN user_capabilities uc ON uc.user_id=other.user_id AND uc.capability='FREELANCER' WHERE other.conversation_id=c.id AND other.user_id<>$2) THEN 'FREELANCER' ELSE 'CUSTOMER' END FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id LEFT JOIN projects p ON p.id=c.project_id WHERE c.id=$1 AND cm.user_id=$2`, id, a).Scan(&v.ID, &v.Kind, &p, &v.CreatedAt, &v.UpdatedAt, &v.ProjectTitle, &v.CounterpartyName, &v.CounterpartyID, &v.CounterpartyUsername, &v.CounterpartyRole)
	if errors.Is(e, sql.ErrNoRows) {
		return Conversation{}, ErrNotFound
	}
	if p.Valid {
		v.ProjectID = &p.String
	}
	return v, e
}
func (r PostgresRepository) ListMessages(ctx context.Context, a, id string, c *Cursor, l int) (MessagePage, error) {
	if _, e := r.GetConversation(ctx, a, id); e != nil {
		return MessagePage{}, e
	}
	var at, cid any
	if c != nil {
		at, cid = c.At, c.ID
	}
	rows, e := r.DB.QueryContext(ctx, `SELECT id::text,conversation_id::text,sender_user_id::text,type,CASE WHEN deleted_at IS NULL THEN COALESCE(body,'') ELSE '' END,COALESCE(reply_to_message_id::text,''),COALESCE(reply_quote,''),COALESCE(client_message_id::text,''),edited_at,deleted_at,created_at FROM messages WHERE conversation_id=$1 AND($2::timestamptz IS NULL OR(created_at,id)<($2,$3::uuid))ORDER BY created_at DESC,id DESC LIMIT $4`, id, at, cid, l+1)
	if e != nil {
		return MessagePage{}, e
	}
	defer rows.Close()
	items := []Message{}
	for rows.Next() {
		var m Message
		if e = rows.Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Type, &m.Body, &m.ReplyToMessageID, &m.ReplyQuote, &m.ClientMessageID, &m.EditedAt, &m.DeletedAt, &m.CreatedAt); e != nil {
			return MessagePage{}, e
		}
		if m.DeletedAt == nil {
			media, e := r.media(ctx, m.ID)
			if e != nil {
				return MessagePage{}, e
			}
			m.MediaIDs = media
		}
		items = append(items, m)
	}
	p := MessagePage{Items: items}
	if len(items) > l {
		x := items[l-1]
		p.Items = items[:l]
		p.NextCursor = &Cursor{x.CreatedAt, x.ID}
	}
	return p, rows.Err()
}
func (r PostgresRepository) Send(ctx context.Context, a, id string, in MessageInput) (Message, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return Message{}, e
	}
	defer tx.Rollback()
	var member bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)`, id, a).Scan(&member); e != nil {
		return Message{}, e
	}
	if !member {
		return Message{}, ErrNotFound
	}
	if in.ReplyToMessageID != "" {
		var ok bool
		_ = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM messages WHERE id=$1 AND conversation_id=$2)`, in.ReplyToMessageID, id).Scan(&ok)
		if !ok {
			return Message{}, ErrInvalid
		}
	}
	for _, mediaID := range in.MediaIDs {
		var ok bool
		e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM media_objects WHERE id=$1 AND owner_user_id=$2 AND purpose='CHAT' AND uploaded_at IS NOT NULL AND scan_status='CLEAN' AND deleted_at IS NULL)`, mediaID, a).Scan(&ok)
		if e != nil || !ok {
			return Message{}, ErrNotFound
		}
	}
	var mid string
	e = tx.QueryRowContext(ctx, `INSERT INTO messages(conversation_id,sender_user_id,type,body,reply_to_message_id,reply_quote,client_message_id)VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,'')::uuid,NULLIF($6,''),$7)ON CONFLICT(sender_user_id,client_message_id)WHERE client_message_id IS NOT NULL DO UPDATE SET client_message_id=EXCLUDED.client_message_id RETURNING id::text`, id, a, in.Type, in.Body, in.ReplyToMessageID, in.ReplyQuote, in.ClientMessageID).Scan(&mid)
	if e != nil {
		return Message{}, mapDB(e)
	}
	for _, mediaID := range in.MediaIDs {
		_, e = tx.ExecContext(ctx, `INSERT INTO message_media(message_id,media_object_id)VALUES($1,$2)ON CONFLICT DO NOTHING`, mid, mediaID)
		if e != nil {
			return Message{}, e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, id)
	if e == nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,aggregate_type,aggregate_id,event_type,payload)VALUES(gen_random_uuid(),'CONVERSATION',$1,'MESSAGE_CREATED',jsonb_build_object('message_id',$2::text))`, id, mid)
	}
	// Notification and email intent are part of the same durable transaction as the
	// message. If the database accepts the message, recipients must never lose the
	// corresponding notification merely because a best-effort follow-up query failed.
	if e == nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO notifications(dedupe_key,user_id,type,actor_user_id,entity_type,entity_id,payload) SELECT 'message:'||$2::text||':'||cm.user_id::text,cm.user_id,'new_message',$3::uuid,'conversation',$1::uuid,jsonb_build_object('conversation_id',$1::text,'message_id',$2::text) FROM conversation_members cm LEFT JOIN notification_preferences np ON np.user_id=cm.user_id AND np.event_type='new_message' WHERE cm.conversation_id=$1::uuid AND cm.user_id<>$3::uuid AND COALESCE(np.in_app,true) ON CONFLICT(dedupe_key)DO NOTHING`, id, mid, a)
	}
	if e == nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO email_jobs(dedupe_key,user_id,template,payload) SELECT 'message-email:'||$2::text||':'||cm.user_id::text,cm.user_id,'new_message',jsonb_build_object('conversation_id',$1::text,'message_id',$2::text,'preferences_path','/settings/notifications') FROM conversation_members cm LEFT JOIN notification_preferences np ON np.user_id=cm.user_id AND np.event_type='new_message' WHERE cm.conversation_id=$1::uuid AND cm.user_id<>$3::uuid AND COALESCE(np.email,true) ON CONFLICT(dedupe_key)DO NOTHING`, id, mid, a)
	}
	// Messages sent by a marketplace user to the support identity also notify real
	// staff accounts by email. The support system user has a deliberately invalid
	// mailbox and must never be the only operational recipient.
	if e == nil {
		_, e = tx.ExecContext(ctx, `INSERT INTO email_jobs(dedupe_key,user_id,template,payload) SELECT 'support-message-email:'||$2::text||':'||ur.user_id::text,ur.user_id,'new_message',jsonb_build_object('conversation_id',$1::text,'message_id',$2::text,'preferences_path','/x7m4q9k2/support') FROM user_roles ur JOIN users u ON u.id=ur.user_id WHERE EXISTS(SELECT 1 FROM conversation_members sm WHERE sm.conversation_id=$1::uuid AND sm.user_id='00000000-0000-4000-8000-000000000011'::uuid) AND $3::uuid<>'00000000-0000-4000-8000-000000000011'::uuid AND ur.role IN('ADMIN','SUPER_ADMIN','MODERATOR') AND u.status='ACTIVE' AND u.deleted_at IS NULL ON CONFLICT(dedupe_key)DO NOTHING`, id, mid, a)
	}
	if e != nil {
		return Message{}, e
	}
	if e = tx.Commit(); e != nil {
		return Message{}, e
	}
	return r.message(ctx, mid)
}

func (r PostgresRepository) MessageNotificationEvents(ctx context.Context, messageID string) ([]RecipientEvent, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT n.user_id::text,jsonb_build_object('id',n.id::text,'type',n.type,'actor_user_id',n.actor_user_id::text,'entity_type',n.entity_type,'entity_id',n.entity_id::text,'payload',n.payload,'read_at',n.read_at,'created_at',n.created_at) FROM notifications n WHERE n.dedupe_key LIKE 'message:'||$1::text||':%' ORDER BY n.created_at,n.id`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecipientEvent{}
	for rows.Next() {
		var userID string
		var raw []byte
		if err = rows.Scan(&userID, &raw); err != nil {
			return nil, err
		}
		var data map[string]any
		if err = json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
		out = append(out, RecipientEvent{UserID: userID, Data: data})
	}
	return out, rows.Err()
}
func (r PostgresRepository) Edit(ctx context.Context, a, id, body string) (Message, error) {
	res, e := r.DB.ExecContext(ctx, `UPDATE messages SET body=$3,edited_at=now() WHERE id=$1 AND sender_user_id=$2 AND type='TEXT' AND deleted_at IS NULL`, id, a, body)
	if e != nil {
		return Message{}, e
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Message{}, ErrNotFound
	}
	return r.message(ctx, id)
}
func (r PostgresRepository) Delete(ctx context.Context, a, id string) (Message, error) {
	res, e := r.DB.ExecContext(ctx, `UPDATE messages SET body=NULL,deleted_at=COALESCE(deleted_at,now()) WHERE id=$1 AND sender_user_id=$2`, id, a)
	if e != nil {
		return Message{}, e
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return Message{}, ErrNotFound
	}
	return r.message(ctx, id)
}
func (r PostgresRepository) Read(ctx context.Context, a, id, message string) error {
	res, e := r.DB.ExecContext(ctx, `UPDATE conversation_members cm SET last_read_message_id=$3 FROM messages target WHERE cm.conversation_id=$1 AND cm.user_id=$2 AND target.id=$3 AND target.conversation_id=$1 AND(cm.last_read_message_id IS NULL OR EXISTS(SELECT 1 FROM messages previous WHERE previous.id=cm.last_read_message_id AND(previous.created_at,previous.id)<(target.created_at,target.id)))`, id, a, message)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var member bool
		_ = r.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)`, id, a).Scan(&member)
		if !member {
			return ErrNotFound
		}
	}
	return nil
}
func (r PostgresRepository) Members(ctx context.Context, id string) ([]string, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT user_id::text FROM conversation_members WHERE conversation_id=$1`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []string{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		v = append(v, id)
	}
	return v, rows.Err()
}
func (r PostgresRepository) message(ctx context.Context, id string) (Message, error) {
	var m Message
	e := r.DB.QueryRowContext(ctx, `SELECT id::text,conversation_id::text,sender_user_id::text,type,CASE WHEN deleted_at IS NULL THEN COALESCE(body,'')ELSE''END,COALESCE(reply_to_message_id::text,''),COALESCE(reply_quote,''),COALESCE(client_message_id::text,''),edited_at,deleted_at,created_at FROM messages WHERE id=$1`, id).Scan(&m.ID, &m.ConversationID, &m.SenderID, &m.Type, &m.Body, &m.ReplyToMessageID, &m.ReplyQuote, &m.ClientMessageID, &m.EditedAt, &m.DeletedAt, &m.CreatedAt)
	if e == nil && m.DeletedAt == nil {
		m.MediaIDs, e = r.media(ctx, id)
	}
	return m, e
}
func (r PostgresRepository) media(ctx context.Context, id string) ([]string, error) {
	rows, e := r.DB.QueryContext(ctx, `SELECT media_object_id::text FROM message_media WHERE message_id=$1`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	v := []string{}
	for rows.Next() {
		var x string
		_ = rows.Scan(&x)
		v = append(v, x)
	}
	return v, rows.Err()
}
func mapDB(e error) error {
	var p *pgconn.PgError
	if errors.As(e, &p) && p.Code == "23505" {
		return ErrConflict
	}
	return e
}

var _ = time.UTC
