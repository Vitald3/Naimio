package analytics

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrUnauthorized = errors.New("analytics authentication required")
	ErrForbidden    = errors.New("analytics entitlement required")
	ErrInvalid      = errors.New("invalid analytics input")
	ErrNotFound     = errors.New("analytics subject not found")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const (
	EventProfileView   = "PROFILE_VIEW"
	EventPortfolioView = "PORTFOLIO_VIEW"
	EventServiceView   = "SERVICE_VIEW"
)

type EventInput struct {
	SubjectUserID string
	ViewerUserID  string
	EventType     string
	EntityType    string
	EntityID      string
	DayKey        string
}

type Metrics struct {
	PeriodDays            int      `json:"period_days"`
	AdvancedUnlocked      bool     `json:"advanced_unlocked"`
	ProSystemEnabled      bool     `json:"pro_system_enabled"`
	ProfileViews          *int64   `json:"profile_views,omitempty"`
	PortfolioViews        *int64   `json:"portfolio_views,omitempty"`
	ServiceViews          *int64   `json:"service_views,omitempty"`
	ProposalsSent         int64    `json:"proposals_sent"`
	JobApplicationsSent   int64    `json:"job_applications_sent"`
	ProfileToProposalRate *float64 `json:"profile_to_proposal_rate,omitempty"`
	LockedAdvancedMetrics []string `json:"locked_advanced_metrics,omitempty"`
}

type EntitlementResolver interface {
	HasAnalytics(context.Context, string) (proSystemEnabled bool, unlocked bool, err error)
}

type Repository interface {
	Record(context.Context, EventInput) error
	ResolveSubjectUserID(context.Context, string, string) (string, error)
	CountEvents(context.Context, string, string, time.Time) (int64, error)
	CountProposals(context.Context, string, time.Time) (int64, error)
	CountJobApplications(context.Context, string, time.Time) (int64, error)
}

type Service struct {
	Repository   Repository
	Entitlements EntitlementResolver
	Now          func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) Track(ctx context.Context, in EventInput) error {
	in.EventType = strings.ToUpper(strings.TrimSpace(in.EventType))
	in.EntityType = strings.ToUpper(strings.TrimSpace(in.EntityType))
	in.SubjectUserID = strings.ToLower(strings.TrimSpace(in.SubjectUserID))
	in.ViewerUserID = strings.ToLower(strings.TrimSpace(in.ViewerUserID))
	in.EntityID = strings.ToLower(strings.TrimSpace(in.EntityID))
	switch in.EventType {
	case EventProfileView:
		in.EntityType = "PROFILE"
	case EventPortfolioView:
		in.EntityType = "PORTFOLIO"
	case EventServiceView:
		in.EntityType = "SERVICE"
		if !uuidPattern.MatchString(in.EntityID) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if in.SubjectUserID == "" {
		if in.EntityType == "SERVICE" {
			id, err := s.Repository.ResolveSubjectUserID(ctx, "SERVICE", in.EntityID)
			if err != nil {
				return err
			}
			in.SubjectUserID = id
		} else {
			return ErrInvalid
		}
	}
	if !uuidPattern.MatchString(in.SubjectUserID) {
		return ErrInvalid
	}
	if in.ViewerUserID != "" && !uuidPattern.MatchString(in.ViewerUserID) {
		return ErrInvalid
	}
	if in.ViewerUserID != "" && in.ViewerUserID == in.SubjectUserID {
		return nil
	}
	in.DayKey = s.now().UTC().Format("2006-01-02")
	return s.Repository.Record(ctx, in)
}

func (s Service) Mine(ctx context.Context, actor string) (Metrics, error) {
	if actor == "" {
		return Metrics{}, ErrUnauthorized
	}
	enabled, unlocked, err := s.Entitlements.HasAnalytics(ctx, actor)
	if err != nil {
		return Metrics{}, err
	}
	since := s.now().Add(-30 * 24 * time.Hour)
	proposals, err := s.Repository.CountProposals(ctx, actor, since)
	if err != nil {
		return Metrics{}, err
	}
	applications, err := s.Repository.CountJobApplications(ctx, actor, since)
	if err != nil {
		return Metrics{}, err
	}
	out := Metrics{
		PeriodDays:          30,
		AdvancedUnlocked:    unlocked,
		ProSystemEnabled:    enabled,
		ProposalsSent:       proposals,
		JobApplicationsSent: applications,
	}
	if !unlocked {
		out.LockedAdvancedMetrics = []string{"profile_views", "portfolio_views", "service_views", "profile_to_proposal_rate"}
		return out, nil
	}
	profileViews, err := s.Repository.CountEvents(ctx, actor, EventProfileView, since)
	if err != nil {
		return Metrics{}, err
	}
	portfolioViews, err := s.Repository.CountEvents(ctx, actor, EventPortfolioView, since)
	if err != nil {
		return Metrics{}, err
	}
	serviceViews, err := s.Repository.CountEvents(ctx, actor, EventServiceView, since)
	if err != nil {
		return Metrics{}, err
	}
	out.ProfileViews = &profileViews
	out.PortfolioViews = &portfolioViews
	out.ServiceViews = &serviceViews
	if profileViews > 0 {
		rate := float64(proposals) / float64(profileViews)
		out.ProfileToProposalRate = &rate
	}
	return out, nil
}
