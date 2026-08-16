package email

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

type Job struct {
	ID, UserID, Template, Address, Locale string
	Payload                               map[string]any
	Attempts                              int
}
type Repository interface {
	Claim(context.Context) (*Job, error)
	Sent(context.Context, string) error
	Retry(context.Context, string, int, string, bool) error
}
type Provider interface {
	Send(context.Context, string, string, string) error
}
type Processor struct {
	Repository    Repository
	Provider      Provider
	PublicBaseURL string
	MaxAttempts   int
}

func (p Processor) Once(ctx context.Context) error {
	j, err := p.Repository.Claim(ctx)
	if err != nil || j == nil {
		return err
	}
	subject, body, err := render(j, p.PublicBaseURL)
	if err == nil {
		err = p.Provider.Send(ctx, j.Address, subject, body)
	}
	if err == nil {
		return p.Repository.Sent(ctx, j.ID)
	}
	attempt := j.Attempts + 1
	final := attempt >= p.max()
	return p.Repository.Retry(ctx, j.ID, attempt, errorCode(err), final)
}
func (p Processor) max() int {
	if p.MaxAttempts < 1 {
		return 5
	}
	return p.MaxAttempts
}
func render(j *Job, base string) (string, string, error) {
	base = strings.TrimRight(base, "/")
	switch j.Template {
	case "verify_email":
		token, _ := j.Payload["token"].(string)
		if token == "" {
			return "", "", errors.New("missing_verification_token")
		}
		return "Подтвердите email", fmt.Sprintf("Подтвердите адрес электронной почты: %s/verify-email?token=%s\n\nСсылка действует 24 часа. Если вы не создавали аккаунт, просто проигнорируйте письмо.", base, token), nil
	case "new_message":
		return "Новое сообщение", fmt.Sprintf("У вас новое сообщение. Откройте диалог: %s/messages\n\nНастройки уведомлений: %s/settings/notifications", base, base), nil
	case "proposal_received":
		return "Новый отклик", fmt.Sprintf("На ваш проект поступил новый отклик: %s/dashboard/projects\n\nНастройки уведомлений: %s/settings/notifications", base, base), nil
	case "project_status_changed":
		return "Статус проекта изменён", fmt.Sprintf("Статус проекта изменился. Подробнее: %s/dashboard/projects\n\nНастройки уведомлений: %s/settings/notifications", base, base), nil
	case "new_review_received":
		return "Новый отзыв", fmt.Sprintf("Вы получили новый отзыв: %s/profile\n\nНастройки уведомлений: %s/settings/notifications", base, base), nil
	case "invite_accepted":
		return "Приглашение принято", fmt.Sprintf("Ваше приглашение принято. Откройте платформу: %s/dashboard/invites\n\nНастройки уведомлений: %s/settings/notifications", base, base), nil
	case "reward_granted":
		return "Промо-награда начислена", fmt.Sprintf("Вам начислена промо-награда. Подробнее: %s/dashboard/invites\n\nНастройки уведомлений: %s/settings/notifications", base, base), nil
	case "invited_to_project":
		return "Приглашение в проект", fmt.Sprintf("Вас пригласили к новому проекту. Откройте приглашения: %s/dashboard/invites\n\nНастройки уведомлений: %s/settings/notifications", base, base), nil
	case "safe_deal_update":
		return "Обновление Безопасной сделки", fmt.Sprintf("Статус Безопасной сделки изменился. Проверьте детали: %s/dashboard/safe-deals\n\nНастройки уведомлений: %s/settings/notifications", base, base), nil
	case "moderation_update":
		title, _ := j.Payload["title"].(string)
		action, _ := j.Payload["action"].(string)
		reason, _ := j.Payload["reason_text"].(string)
		editURL, _ := j.Payload["edit_url"].(string)
		if title == "" {
			title = "Материал"
		}
		body := fmt.Sprintf("Решение модерации по материалу «%s»: %s.", title, action)
		if reason != "" {
			body += "\nПричина: " + reason
		}
		if editURL != "" {
			body += "\nИсправить материал: " + base + editURL
		}
		body += "\n\nУведомления: " + base + "/notifications"
		return "Решение модерации — Naimio", body, nil
	case "new_project_available":
		return "Новые проекты на Naimio", fmt.Sprintf("В новой подборке: %v. Посмотрите задачи и бюджеты: %s/projects\n\nНастройки рассылки: %s/settings/notifications", j.Payload["count"], base, base), nil
	case "new_vacancy_available":
		return "Новые вакансии на Naimio", fmt.Sprintf("В новой подборке: %v. Посмотрите требования и условия: %s/vacancies\n\nНастройки рассылки: %s/settings/notifications", j.Payload["count"], base, base), nil
	case "new_service_available":
		return "Новые услуги на Naimio", fmt.Sprintf("В новой подборке: %v. Посмотрите услуги, консультации и обучение: %s/services\n\nНастройки рассылки: %s/settings/notifications", j.Payload["count"], base, base), nil
	default:
		return "", "", errors.New("unknown_template")
	}
}
func errorCode(err error) string {
	if err == nil {
		return ""
	}
	v := err.Error()
	if len(v) > 100 {
		v = v[:100]
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, v)
}

type PostgresRepository struct{ DB *sql.DB }

func (r PostgresRepository) Claim(ctx context.Context) (*Job, error) {
	tx, e := r.DB.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var j Job
	var raw []byte
	e = tx.QueryRowContext(ctx, `SELECT ej.id::text,ej.user_id::text,ej.template,ej.payload,ej.attempts,u.email,u.locale FROM email_jobs ej JOIN users u ON u.id=ej.user_id WHERE ej.status IN('PENDING','FAILED') AND ej.available_at<=now() AND ej.attempts<10 ORDER BY ej.available_at,ej.id FOR UPDATE OF ej SKIP LOCKED LIMIT 1`).Scan(&j.ID, &j.UserID, &j.Template, &raw, &j.Attempts, &j.Address, &j.Locale)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(raw, &j.Payload); e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE email_jobs SET status='SENDING' WHERE id=$1`, j.ID); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return &j, nil
}
func (r PostgresRepository) Sent(ctx context.Context, id string) error {
	_, e := r.DB.ExecContext(ctx, `UPDATE email_jobs SET status='SENT',sent_at=now(),last_error_code=NULL WHERE id=$1`, id)
	return e
}
func (r PostgresRepository) Retry(ctx context.Context, id string, attempt int, code string, final bool) error {
	status := "FAILED"
	delay := time.Duration(1<<min(attempt, 6)) * time.Minute
	if final {
		status = "DEAD"
	}
	_, e := r.DB.ExecContext(ctx, `UPDATE email_jobs SET status=$2,attempts=$3,last_error_code=$4,available_at=now()+$5::interval WHERE id=$1`, id, status, attempt, code, delay.String())
	return e
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type SMTPProvider struct{ Address, Username, Password, From string }

func (p SMTPProvider) Send(_ context.Context, to, subject, body string) error {
	host := strings.Split(p.Address, ":")[0]
	var auth smtp.Auth
	if p.Username != "" {
		auth = smtp.PlainAuth("", p.Username, p.Password, host)
	}
	msg := []byte("From: " + p.From + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body)
	return smtp.SendMail(p.Address, auth, p.From, []string{to}, msg)
}
