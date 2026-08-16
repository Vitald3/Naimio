package catalog

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
)

var (
	ErrNotFound  = errors.New("catalog item not found")
	ErrForbidden = errors.New("admin role required")
	ErrInvalid   = errors.New("invalid catalog input")
)
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Category struct {
	ID          string     `json:"id"`
	ParentID    *string    `json:"parent_id,omitempty"`
	Slug        string     `json:"slug"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	SortOrder   int        `json:"sort_order"`
	Active      bool       `json:"is_active"`
	Children    []Category `json:"children,omitempty"`
}
type Skill struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Active bool   `json:"is_active"`
}
type CategoryInput struct {
	ParentID    *string `json:"parent_id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	SortOrder   int     `json:"sort_order"`
	Active      bool    `json:"is_active"`
}
type SkillInput struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Active bool   `json:"is_active"`
}

type Repository interface {
	CategoryTree(context.Context) ([]Category, error)
	Category(context.Context, string) (Category, error)
	SearchSkills(context.Context, string) ([]Skill, error)
	AdminCategories(context.Context, string) ([]Category, error)
	AdminSkills(context.Context, string) ([]Skill, error)
	CreateCategory(context.Context, string, CategoryInput) (Category, error)
	UpdateCategory(context.Context, string, string, CategoryInput) (Category, error)
	DeleteCategory(context.Context, string, string) error
	CreateSkill(context.Context, string, SkillInput) (Skill, error)
	UpdateSkill(context.Context, string, string, SkillInput) (Skill, error)
	DeleteSkill(context.Context, string, string) error
}

type Store struct {
	Categories []Category
	Skills     []Skill
	Admins     map[string]bool
}

func (s Store) CategoryTree(context.Context) ([]Category, error) { return buildTree(s.Categories), nil }
func (s Store) Category(_ context.Context, slug string) (Category, error) {
	for _, c := range s.Categories {
		if c.Active && c.Slug == slug {
			c.Children = childrenOf(c.ID, s.Categories, 1)
			return c, nil
		}
	}
	return Category{}, ErrNotFound
}
func (s Store) SearchSkills(_ context.Context, query string) ([]Skill, error) {
	if len([]rune(query)) > 100 {
		return nil, ErrInvalid
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := []Skill{}
	for _, skill := range s.Skills {
		if skill.Active && (q == "" || strings.Contains(strings.ToLower(skill.Name), q) || strings.Contains(strings.ToLower(skill.Slug), q)) {
			out = append(out, skill)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > 50 {
		out = out[:50]
	}
	return out, nil
}
func (s Store) AdminCategories(_ context.Context, actor string) ([]Category, error) {
	if !s.Admins[actor] {
		return nil, ErrForbidden
	}
	out := append([]Category(nil), s.Categories...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
func (s Store) AdminSkills(_ context.Context, actor string) ([]Skill, error) {
	if !s.Admins[actor] {
		return nil, ErrForbidden
	}
	out := append([]Skill(nil), s.Skills...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func (s Store) CreateCategory(context.Context, string, CategoryInput) (Category, error) {
	return Category{}, errors.New("store admin mutation unavailable")
}
func (s Store) UpdateCategory(context.Context, string, string, CategoryInput) (Category, error) {
	return Category{}, errors.New("store admin mutation unavailable")
}
func (s Store) DeleteCategory(context.Context, string, string) error {
	return errors.New("store admin mutation unavailable")
}
func (s Store) CreateSkill(context.Context, string, SkillInput) (Skill, error) {
	return Skill{}, errors.New("store admin mutation unavailable")
}
func (s Store) UpdateSkill(context.Context, string, string, SkillInput) (Skill, error) {
	return Skill{}, errors.New("store admin mutation unavailable")
}
func (s Store) DeleteSkill(context.Context, string, string) error {
	return errors.New("store admin mutation unavailable")
}

func buildTree(all []Category) []Category {
	roots := []Category{}
	for _, c := range all {
		if c.Active && c.ParentID == nil {
			c.Children = childrenOf(c.ID, all, 1)
			roots = append(roots, c)
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].SortOrder != roots[j].SortOrder {
			return roots[i].SortOrder < roots[j].SortOrder
		}
		return roots[i].Name < roots[j].Name
	})
	return roots
}
func childrenOf(parent string, all []Category, depth int) []Category {
	if depth >= 3 {
		return nil
	}
	out := []Category{}
	for _, c := range all {
		if c.Active && c.ParentID != nil && *c.ParentID == parent {
			c.Children = childrenOf(c.ID, all, depth+1)
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out
}
func validInput(slug, name string) bool {
	slug = strings.TrimSpace(slug)
	name = strings.TrimSpace(name)
	if len(slug) < 1 || len(slug) > 120 || len([]rune(name)) < 1 || len([]rune(name)) > 160 {
		return false
	}
	for _, r := range slug {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
func validCategoryInput(in CategoryInput) bool {
	return validInput(in.Slug, in.Name) && len([]rune(in.Description)) <= 2000 && in.SortOrder >= 0 && in.SortOrder <= 10000 && (in.ParentID == nil || uuidPattern.MatchString(strings.ToLower(strings.TrimSpace(*in.ParentID))))
}
