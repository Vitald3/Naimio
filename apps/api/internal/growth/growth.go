package growth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrUnauthorized = errors.New("authentication required")
	ErrNotFound     = errors.New("growth resource not found")
	ErrInvalid      = errors.New("invalid growth input")
	ErrConflict     = errors.New("growth conflict")
	ErrForbidden    = errors.New("admin role required")
)
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type InviteInput struct {
	Type          string `json:"type"`
	ProjectID     string `json:"project_id"`
	IntendedEmail string `json:"intended_email"`
}
type Invite struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	ProjectID  *string    `json:"project_id,omitempty"`
	AcceptedBy *string    `json:"accepted_by_user_id,omitempty"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
}
type CreatedInvite struct {
	Invite Invite `json:"invite"`
	Token  string `json:"token"`
	URL    string `json:"url"`
}
type Preview struct {
	Type               string    `json:"type"`
	ProjectID          *string   `json:"project_id,omitempty"`
	ProjectTitle       string    `json:"project_title,omitempty"`
	CategoryName       string    `json:"category_name,omitempty"`
	InviterDisplayName string    `json:"inviter_display_name"`
	InvitedRole        string    `json:"invited_role"`
	ExpiresAt          time.Time `json:"expires_at"`
	Accepted           bool      `json:"accepted"`
}
type Acceptance struct {
	InviteID           string    `json:"invite_id"`
	Type               string    `json:"type"`
	ProjectID          *string   `json:"project_id,omitempty"`
	AcceptedAt         time.Time `json:"accepted_at"`
	AttributionCreated bool      `json:"attribution_created"`
	RewardsIssued      int       `json:"rewards_issued"`
}
type Attribution struct {
	ID            string    `json:"id"`
	InviterUserID string    `json:"inviter_user_id"`
	InviteID      string    `json:"invite_id"`
	Source        string    `json:"source"`
	FirstTouchAt  time.Time `json:"first_touch_at"`
}
type Reward struct {
	ID         string     `json:"id"`
	RuleCode   string     `json:"rule_code"`
	EventKey   string     `json:"event_key"`
	RewardType string     `json:"reward_type"`
	Amount     int64      `json:"amount"`
	Unit       string     `json:"unit"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}
type Referrals struct {
	Attribution *Attribution `json:"attribution,omitempty"`
	Rewards     []Reward     `json:"rewards"`
}
type RuleInput struct {
	Code        string         `json:"code"`
	EventType   string         `json:"event_type"`
	Beneficiary string         `json:"beneficiary"`
	RewardType  string         `json:"reward_type"`
	RewardValue int64          `json:"reward_value"`
	RewardUnit  string         `json:"reward_unit"`
	MaxUses     *int           `json:"max_uses"`
	StartsAt    *time.Time     `json:"starts_at"`
	EndsAt      *time.Time     `json:"ends_at"`
	Enabled     bool           `json:"enabled"`
	Config      map[string]any `json:"config"`
}
type Rule struct {
	ID string `json:"id"`
	RuleInput
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type TeamInput struct {
	Label string `json:"label"`
	Notes string `json:"notes"`
}
type TeamMember struct {
	FreelancerUserID  string    `json:"freelancer_user_id"`
	Username          string    `json:"username,omitempty"`
	DisplayName       string    `json:"display_name"`
	Availability      string    `json:"availability"`
	ProfessionalTitle string    `json:"professional_title,omitempty"`
	NativeRating      *float64  `json:"native_rating,omitempty"`
	ReviewsCount      int       `json:"reviews_count"`
	Label             string    `json:"label,omitempty"`
	Notes             string    `json:"notes,omitempty"`
	LastProjectID     string    `json:"last_project_id,omitempty"`
	LastProjectTitle  string    `json:"last_project_title,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
type RepeatInput struct {
	InvitePreviousFreelancer bool `json:"invite_previous_freelancer"`
}
type RepeatResult struct {
	ProjectID       string         `json:"project_id"`
	SourceProjectID string         `json:"source_project_id"`
	Status          string         `json:"status"`
	SourceType      string         `json:"source_type"`
	Invite          *CreatedInvite `json:"invite,omitempty"`
}
type ShareResult struct {
	ProjectID string `json:"project_id"`
	URL       string `json:"url"`
}
type InvitedProject struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	Description         string `json:"description"`
	CategoryName        string `json:"category_name,omitempty"`
	Visibility          string `json:"visibility"`
	Status              string `json:"status"`
	CustomerDisplayName string `json:"customer_display_name"`
}
type Repository interface {
	CreateInvite(context.Context, string, InviteInput, []byte, time.Time) (Invite, error)
	Preview(context.Context, []byte) (Preview, error)
	Accept(context.Context, string, []byte, string) (Acceptance, error)
	Referrals(context.Context, string) (Referrals, error)
	Rules(context.Context, string) ([]Rule, error)
	CreateRule(context.Context, string, RuleInput) (Rule, error)
	UpdateRule(context.Context, string, string, RuleInput) (Rule, error)
	Team(context.Context, string) ([]TeamMember, error)
	PutTeam(context.Context, string, string, TeamInput) (TeamMember, error)
	DeleteTeam(context.Context, string, string) error
	Repeat(context.Context, string, string, RepeatInput, []byte, time.Time) (RepeatResult, error)
	Share(context.Context, string, string, string) (ShareResult, error)
	InvitedProject(context.Context, string, string) (InvitedProject, error)
}
type Service struct {
	Repository    Repository
	PublicBaseURL string
	Now           func() time.Time
}

func (s Service) CreateInvite(ctx context.Context, actor string, in InviteInput) (CreatedInvite, error) {
	if actor == "" {
		return CreatedInvite{}, ErrUnauthorized
	}
	in = normalizeInvite(in)
	if !validInvite(in) {
		return CreatedInvite{}, ErrInvalid
	}
	token, hash, err := newToken()
	if err != nil {
		return CreatedInvite{}, err
	}
	expires := s.now().Add(14 * 24 * time.Hour)
	v, err := s.Repository.CreateInvite(ctx, actor, in, hash, expires)
	if err != nil {
		return CreatedInvite{}, err
	}
	return CreatedInvite{Invite: v, Token: token, URL: strings.TrimRight(s.PublicBaseURL, "/") + "/invite/" + token}, nil
}
func (s Service) Preview(ctx context.Context, token string) (Preview, error) {
	hash, ok := tokenHash(token)
	if !ok {
		return Preview{}, ErrNotFound
	}
	return s.Repository.Preview(ctx, hash)
}
func (s Service) Accept(ctx context.Context, actor, token, key string) (Acceptance, error) {
	if actor == "" {
		return Acceptance{}, ErrUnauthorized
	}
	if len(key) < 8 || len(key) > 128 {
		return Acceptance{}, ErrInvalid
	}
	hash, ok := tokenHash(token)
	if !ok {
		return Acceptance{}, ErrNotFound
	}
	return s.Repository.Accept(ctx, actor, hash, key)
}
func (s Service) Referrals(ctx context.Context, actor string) (Referrals, error) {
	if actor == "" {
		return Referrals{}, ErrUnauthorized
	}
	return s.Repository.Referrals(ctx, actor)
}
func (s Service) Rules(ctx context.Context, actor string) ([]Rule, error) {
	if actor == "" {
		return nil, ErrUnauthorized
	}
	return s.Repository.Rules(ctx, actor)
}
func (s Service) CreateRule(ctx context.Context, actor string, in RuleInput) (Rule, error) {
	in = normalizeRule(in)
	if !validRule(in) {
		return Rule{}, ErrInvalid
	}
	return s.Repository.CreateRule(ctx, actor, in)
}
func (s Service) UpdateRule(ctx context.Context, actor, id string, in RuleInput) (Rule, error) {
	in = normalizeRule(in)
	if !uuid(id) || !validRule(in) {
		return Rule{}, ErrInvalid
	}
	return s.Repository.UpdateRule(ctx, actor, id, in)
}
func (s Service) Team(ctx context.Context, actor string) ([]TeamMember, error) {
	if actor == "" {
		return nil, ErrUnauthorized
	}
	return s.Repository.Team(ctx, actor)
}
func (s Service) PutTeam(ctx context.Context, actor, freelancer string, in TeamInput) (TeamMember, error) {
	in.Label = strings.TrimSpace(in.Label)
	in.Notes = strings.TrimSpace(in.Notes)
	if actor == "" {
		return TeamMember{}, ErrUnauthorized
	}
	if !uuid(freelancer) || freelancer == actor || len([]rune(in.Label)) > 120 || len([]rune(in.Notes)) > 2000 {
		return TeamMember{}, ErrInvalid
	}
	return s.Repository.PutTeam(ctx, actor, freelancer, in)
}
func (s Service) DeleteTeam(ctx context.Context, actor, freelancer string) error {
	if actor == "" {
		return ErrUnauthorized
	}
	if !uuid(freelancer) {
		return ErrNotFound
	}
	return s.Repository.DeleteTeam(ctx, actor, freelancer)
}
func (s Service) Repeat(ctx context.Context, actor, project string, in RepeatInput) (RepeatResult, error) {
	if actor == "" {
		return RepeatResult{}, ErrUnauthorized
	}
	if !uuid(project) {
		return RepeatResult{}, ErrNotFound
	}
	var hash []byte
	var token string
	var err error
	expires := s.now().Add(14 * 24 * time.Hour)
	if in.InvitePreviousFreelancer {
		token, hash, err = newToken()
		if err != nil {
			return RepeatResult{}, err
		}
	}
	v, err := s.Repository.Repeat(ctx, actor, project, in, hash, expires)
	if err != nil {
		return RepeatResult{}, err
	}
	if v.Invite != nil {
		v.Invite.Token = token
		v.Invite.URL = strings.TrimRight(s.PublicBaseURL, "/") + "/invite/" + token
	}
	return v, nil
}
func (s Service) Share(ctx context.Context, actor, project string) (ShareResult, error) {
	if actor == "" {
		return ShareResult{}, ErrUnauthorized
	}
	if !uuid(project) {
		return ShareResult{}, ErrNotFound
	}
	return s.Repository.Share(ctx, actor, project, strings.TrimRight(s.PublicBaseURL, "/"))
}
func (s Service) InvitedProject(ctx context.Context, actor, project string) (InvitedProject, error) {
	if actor == "" {
		return InvitedProject{}, ErrUnauthorized
	}
	if !uuid(project) {
		return InvitedProject{}, ErrNotFound
	}
	return s.Repository.InvitedProject(ctx, actor, project)
}
func normalizeInvite(v InviteInput) InviteInput {
	v.Type = strings.ToUpper(strings.TrimSpace(v.Type))
	v.ProjectID = strings.ToLower(strings.TrimSpace(v.ProjectID))
	v.IntendedEmail = strings.ToLower(strings.TrimSpace(v.IntendedEmail))
	return v
}
func validInvite(v InviteInput) bool {
	if v.Type != "CUSTOMER" && v.Type != "FREELANCER" && v.Type != "PROJECT" {
		return false
	}
	if v.Type != "CUSTOMER" && !uuid(v.ProjectID) {
		return false
	}
	if v.ProjectID != "" && !uuid(v.ProjectID) {
		return false
	}
	return v.IntendedEmail == "" || len(v.IntendedEmail) <= 320 && strings.Count(v.IntendedEmail, "@") == 1 && !strings.ContainsAny(v.IntendedEmail, " \t\r\n")
}
func normalizeRule(v RuleInput) RuleInput {
	v.Code = strings.ToUpper(strings.TrimSpace(v.Code))
	v.EventType = strings.ToUpper(strings.TrimSpace(v.EventType))
	v.Beneficiary = strings.ToUpper(strings.TrimSpace(v.Beneficiary))
	v.RewardType = strings.ToUpper(strings.TrimSpace(v.RewardType))
	v.RewardUnit = strings.ToUpper(strings.TrimSpace(v.RewardUnit))
	if v.Config == nil {
		v.Config = map[string]any{}
	}
	return v
}
func validRule(v RuleInput) bool {
	if len(v.Code) < 1 || len(v.Code) > 100 || !regexp.MustCompile(`^[A-Z0-9_]+$`).MatchString(v.Code) || v.EventType != "INVITE_ACCEPTED" || (v.Beneficiary != "INVITER" && v.Beneficiary != "INVITED") || v.RewardValue < 1 {
		return false
	}
	if v.RewardType != "COMMISSION_DISCOUNT" && v.RewardType != "BONUS" && v.RewardType != "FIXED_REWARD" && v.RewardType != "PERCENT_REWARD" {
		return false
	}
	if v.RewardUnit != "KOPECKS" && v.RewardUnit != "BASIS_POINTS" && v.RewardUnit != "COUNT" && v.RewardUnit != "CREDITS" {
		return false
	}
	if v.RewardUnit == "BASIS_POINTS" && v.RewardValue > 10000 || v.RewardType == "PERCENT_REWARD" && v.RewardUnit != "BASIS_POINTS" || v.MaxUses != nil && *v.MaxUses < 1 {
		return false
	}
	if days, ok := v.Config["valid_days"]; ok {
		number, ok := days.(float64)
		if !ok || number < 1 || number > 3650 || number != float64(int(number)) {
			return false
		}
	}
	return v.StartsAt == nil || v.EndsAt == nil || v.StartsAt.Before(*v.EndsAt)
}
func newToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	return token, sum[:], nil
}
func tokenHash(token string) ([]byte, bool) {
	if len(token) != 43 {
		return nil, false
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) != 32 {
		return nil, false
	}
	sum := sha256.Sum256([]byte(token))
	return sum[:], true
}
func uuid(v string) bool { return uuidPattern.MatchString(strings.ToLower(v)) }
func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
