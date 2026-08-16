package catalog

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestCategoryTreeAndSkillSearch(t *testing.T) {
	parent := "cat-1"
	s := Store{Categories: []Category{{ID: parent, Slug: "dev", Name: "Разработка", Active: true}, {ID: "cat-2", ParentID: &parent, Slug: "go", Name: "Go", Active: true}}, Skills: []Skill{{ID: "skill-1", Slug: "go", Name: "Go", Active: true}, {ID: "skill-2", Slug: "figma", Name: "Figma", Active: true}}}
	tree, err := s.CategoryTree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(tree[0].Children); got != 1 {
		t.Fatalf("children = %d", got)
	}
	got, err := s.SearchSkills(context.Background(), "fig")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Slug != "figma" {
		t.Fatalf("skills = %#v", got)
	}
}

func TestSkillsEndpointUsesJSONContract(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/skills?q=go", nil)
	res := httptest.NewRecorder()
	(Handler{Repository: Store{Skills: []Skill{{ID: "1", Slug: "go", Name: "Go", Active: true}}}}).Skills(res, req)
	if res.Code != 200 || res.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("response = %d %q", res.Code, res.Header().Get("Content-Type"))
	}
}
