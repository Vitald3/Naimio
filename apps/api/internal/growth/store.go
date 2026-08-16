package growth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
	"time"
)

type StoreProject struct{ ID, Owner, Title, Category, Slug, Status, Visibility, PreviousFreelancer string }
type StoredInvite struct {
	Invite
	Inviter, IntendedEmail string
	Hash                   []byte
}
type Store struct {
	Mu           sync.Mutex
	Invites      map[string]StoredInvite
	Users        map[string]bool
	Emails       map[string]string
	Capabilities map[string]map[string]bool
	Projects     map[string]StoreProject
	Attributions map[string]Attribution
	Rewards      map[string]Reward
	RulesMap     map[string]Rule
	Admins       map[string]bool
	TeamMap      map[string]TeamMember
	FraudSignals []string
	Invited      map[string]bool
	Now          func() time.Time
}

func (s *Store) CreateInvite(_ context.Context, a string, in InviteInput, hash []byte, expires time.Time) (Invite, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if !s.Users[a] {
		return Invite{}, ErrUnauthorized
	}
	if in.IntendedEmail != "" && s.Emails[a] == in.IntendedEmail {
		s.FraudSignals = append(s.FraudSignals, "SELF_REFERRAL")
		return Invite{}, ErrInvalid
	}
	if !s.allowed(a, in) {
		return Invite{}, ErrNotFound
	}
	id := randomID()
	var project *string
	if in.ProjectID != "" {
		v := in.ProjectID
		project = &v
	}
	v := Invite{ID: id, Type: in.Type, ProjectID: project, ExpiresAt: expires, CreatedAt: s.now()}
	if s.Invites == nil {
		s.Invites = map[string]StoredInvite{}
	}
	s.Invites[hex.EncodeToString(hash)] = StoredInvite{Invite: v, Inviter: a, IntendedEmail: in.IntendedEmail, Hash: hash}
	return v, nil
}
func (s *Store) Preview(_ context.Context, hash []byte) (Preview, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	v, ok := s.Invites[hex.EncodeToString(hash)]
	if !ok || s.now().After(v.ExpiresAt) {
		return Preview{}, ErrNotFound
	}
	p := Preview{Type: v.Type, ProjectID: v.ProjectID, InviterDisplayName: "Inviter", InvitedRole: invitedRole(v.Type), ExpiresAt: v.ExpiresAt, Accepted: v.AcceptedAt != nil}
	if v.ProjectID != nil {
		project := s.Projects[*v.ProjectID]
		p.ProjectTitle, p.CategoryName = project.Title, project.Category
	}
	return p, nil
}
func (s *Store) Accept(_ context.Context, a string, hash []byte, _ string) (Acceptance, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	key := hex.EncodeToString(hash)
	v, ok := s.Invites[key]
	if !ok || s.now().After(v.ExpiresAt) {
		return Acceptance{}, ErrNotFound
	}
	if v.Inviter == a {
		s.FraudSignals = append(s.FraudSignals, "SELF_REFERRAL")
		return Acceptance{}, ErrInvalid
	}
	if v.IntendedEmail != "" && s.Emails[a] != v.IntendedEmail {
		return Acceptance{}, ErrNotFound
	}
	if v.AcceptedBy != nil {
		if *v.AcceptedBy != a {
			return Acceptance{}, ErrNotFound
		}
		return Acceptance{InviteID: v.ID, Type: v.Type, ProjectID: v.ProjectID, AcceptedAt: *v.AcceptedAt}, nil
	}
	if !s.Users[a] {
		return Acceptance{}, ErrUnauthorized
	}
	now := s.now()
	v.AcceptedBy = &a
	v.AcceptedAt = &now
	s.Invites[key] = v
	if s.Capabilities[a] == nil {
		s.Capabilities[a] = map[string]bool{}
	}
	if v.Type == "CUSTOMER" {
		s.Capabilities[a]["CUSTOMER"] = true
	} else {
		s.Capabilities[a]["FREELANCER"] = true
		if v.ProjectID != nil {
			if s.Invited == nil {
				s.Invited = map[string]bool{}
			}
			s.Invited[*v.ProjectID+":"+a] = true
		}
	}
	created := false
	if s.Attributions == nil {
		s.Attributions = map[string]Attribution{}
	}
	if _, exists := s.Attributions[a]; !exists {
		s.Attributions[a] = Attribution{ID: randomID(), InviterUserID: v.Inviter, InviteID: v.ID, Source: "INVITE", FirstTouchAt: now}
		created = true
	}
	issued := 0
	for _, rule := range s.RulesMap {
		if !rule.Enabled || rule.EventType != "INVITE_ACCEPTED" || rule.StartsAt != nil && now.Before(*rule.StartsAt) || rule.EndsAt != nil && !now.Before(*rule.EndsAt) {
			continue
		}
		user := a
		if rule.Beneficiary == "INVITER" {
			user = v.Inviter
		}
		event := rule.Code + ":" + v.ID
		ledgerKey := user + ":" + event
		if _, exists := s.Rewards[ledgerKey]; exists {
			continue
		}
		if rule.MaxUses != nil && s.ruleUses(rule.ID) >= *rule.MaxUses {
			continue
		}
		if s.Rewards == nil {
			s.Rewards = map[string]Reward{}
		}
		s.Rewards[ledgerKey] = Reward{ID: randomID(), RuleCode: rule.Code, EventKey: event, RewardType: rule.RewardType, Amount: rule.RewardValue, Unit: rule.RewardUnit, CreatedAt: now}
		issued++
	}
	return Acceptance{InviteID: v.ID, Type: v.Type, ProjectID: v.ProjectID, AcceptedAt: now, AttributionCreated: created, RewardsIssued: issued}, nil
}
func (s *Store) Referrals(_ context.Context, a string) (Referrals, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	var attribution *Attribution
	if v, ok := s.Attributions[a]; ok {
		x := v
		attribution = &x
	}
	rewards := []Reward{}
	for key, v := range s.Rewards {
		if len(key) > len(a) && key[:len(a)] == a {
			rewards = append(rewards, v)
		}
	}
	sort.Slice(rewards, func(i, j int) bool { return rewards[i].CreatedAt.After(rewards[j].CreatedAt) })
	return Referrals{Attribution: attribution, Rewards: rewards}, nil
}
func (s *Store) Rules(_ context.Context, a string) ([]Rule, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if !s.Admins[a] {
		return nil, ErrForbidden
	}
	v := []Rule{}
	for _, r := range s.RulesMap {
		v = append(v, r)
	}
	return v, nil
}
func (s *Store) CreateRule(_ context.Context, a string, in RuleInput) (Rule, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if !s.Admins[a] {
		return Rule{}, ErrForbidden
	}
	for _, v := range s.RulesMap {
		if v.Code == in.Code {
			return Rule{}, ErrConflict
		}
	}
	now := s.now()
	v := Rule{ID: randomID(), RuleInput: in, CreatedAt: now, UpdatedAt: now}
	if s.RulesMap == nil {
		s.RulesMap = map[string]Rule{}
	}
	s.RulesMap[v.ID] = v
	return v, nil
}
func (s *Store) UpdateRule(_ context.Context, a, id string, in RuleInput) (Rule, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if !s.Admins[a] {
		return Rule{}, ErrForbidden
	}
	v, ok := s.RulesMap[id]
	if !ok {
		return Rule{}, ErrNotFound
	}
	v.RuleInput = in
	v.UpdatedAt = s.now()
	s.RulesMap[id] = v
	return v, nil
}
func (s *Store) Team(_ context.Context, a string) ([]TeamMember, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if !s.Capabilities[a]["CUSTOMER"] {
		return nil, ErrNotFound
	}
	v := []TeamMember{}
	for key, item := range s.TeamMap {
		if len(key) > len(a) && key[:len(a)] == a {
			v = append(v, item)
		}
	}
	return v, nil
}
func (s *Store) PutTeam(_ context.Context, a, f string, in TeamInput) (TeamMember, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if !s.Capabilities[a]["CUSTOMER"] || !s.Capabilities[f]["FREELANCER"] {
		return TeamMember{}, ErrNotFound
	}
	now := s.now()
	key := a + ":" + f
	v := s.TeamMap[key]
	if v.CreatedAt.IsZero() {
		v = TeamMember{FreelancerUserID: f, DisplayName: "Freelancer", Availability: "AVAILABLE", CreatedAt: now}
	}
	v.Label, v.Notes, v.UpdatedAt = in.Label, in.Notes, now
	if s.TeamMap == nil {
		s.TeamMap = map[string]TeamMember{}
	}
	s.TeamMap[key] = v
	return v, nil
}
func (s *Store) DeleteTeam(_ context.Context, a, f string) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	delete(s.TeamMap, a+":"+f)
	return nil
}
func (s *Store) Repeat(_ context.Context, a, id string, in RepeatInput, hash []byte, expires time.Time) (RepeatResult, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	source, ok := s.Projects[id]
	if !ok || source.Owner != a {
		return RepeatResult{}, ErrNotFound
	}
	if source.Status != "COMPLETED" {
		return RepeatResult{}, ErrConflict
	}
	newID := randomID()
	s.Projects[newID] = StoreProject{ID: newID, Owner: a, Title: source.Title, Category: source.Category, Slug: source.Slug + "-repeat", Status: "DRAFT", Visibility: source.Visibility}
	result := RepeatResult{ProjectID: newID, SourceProjectID: id, Status: "DRAFT", SourceType: "REPEAT"}
	if in.InvitePreviousFreelancer && source.PreviousFreelancer != "" {
		invite := Invite{ID: randomID(), Type: "FREELANCER", ProjectID: &newID, ExpiresAt: expires, CreatedAt: s.now()}
		s.Invites[hex.EncodeToString(hash)] = StoredInvite{Invite: invite, Inviter: a, IntendedEmail: s.Emails[source.PreviousFreelancer], Hash: hash}
		result.Invite = &CreatedInvite{Invite: invite}
	}
	return result, nil
}
func (s *Store) Share(_ context.Context, a, id, base string) (ShareResult, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	p, ok := s.Projects[id]
	if !ok || p.Owner != a || p.Visibility != "PUBLIC" || (p.Status != "OPEN" && p.Status != "MATCHING") {
		return ShareResult{}, ErrNotFound
	}
	return ShareResult{ProjectID: id, URL: base + "/projects/" + p.Slug}, nil
}
func (s *Store) InvitedProject(_ context.Context, a, id string) (InvitedProject, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	p, ok := s.Projects[id]
	if !ok || !s.Invited[id+":"+a] {
		return InvitedProject{}, ErrNotFound
	}
	return InvitedProject{ID: p.ID, Title: p.Title, CategoryName: p.Category, Visibility: p.Visibility, Status: p.Status, CustomerDisplayName: "Customer"}, nil
}
func (s *Store) allowed(a string, in InviteInput) bool {
	caps := s.Capabilities[a]
	if in.Type == "CUSTOMER" {
		if !caps["FREELANCER"] {
			return false
		}
		if in.ProjectID == "" {
			return true
		}
	} else if !caps["CUSTOMER"] {
		return false
	}
	if in.ProjectID != "" {
		p, ok := s.Projects[in.ProjectID]
		return ok && (p.Owner == a || p.PreviousFreelancer == a)
	}
	return true
}
func invitedRole(kind string) string {
	if kind == "CUSTOMER" {
		return "CUSTOMER"
	}
	return "FREELANCER"
}
func (s *Store) ruleUses(id string) int {
	n := 0
	for _, v := range s.Rewards {
		for _, rule := range s.RulesMap {
			if rule.ID == id && v.RuleCode == rule.Code {
				n++
			}
		}
	}
	return n
}
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
