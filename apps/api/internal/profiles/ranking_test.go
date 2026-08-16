package profiles

import "testing"

func TestBoundedProBoostCapsAndDisables(t *testing.T) {
	if got := BoundedProBoost(false, 1.5); got != 0 {
		t.Fatalf("disabled boost=%v", got)
	}
	if got := BoundedProBoost(true, 1.0); got != 0 {
		t.Fatalf("neutral boost=%v", got)
	}
	if got := BoundedProBoost(true, 1.08); got != 8 {
		t.Fatalf("configured boost=%v", got)
	}
	if got := BoundedProBoost(true, 2.0); got != MaxProBoostPoints {
		t.Fatalf("capped boost=%v", got)
	}
}

func TestDiscoveryScoreGivesPROAdvantageWhenSimilar(t *testing.T) {
	free := DiscoveryScore(50, 20, 0)
	pro := DiscoveryScore(50, 20, BoundedProBoost(true, 1.08))
	if !(pro > free) {
		t.Fatalf("expected PRO advantage free=%v pro=%v", free, pro)
	}
}

func TestDiscoveryScoreLetsRelevanceDominate(t *testing.T) {
	relevantFree := DiscoveryScore(90, 18, 0)
	weakPRO := DiscoveryScore(55, 18, BoundedProBoost(true, 1.08))
	if !(relevantFree > weakPRO) {
		t.Fatalf("relevance must dominate free=%v pro=%v", relevantFree, weakPRO)
	}
}

func TestDiscoveryScoreDisablingPRORemovesBoost(t *testing.T) {
	with := DiscoveryScore(50, 20, BoundedProBoost(true, 1.08))
	without := DiscoveryScore(50, 20, BoundedProBoost(false, 1.08))
	if with <= without || without != 70 {
		t.Fatalf("with=%v without=%v", with, without)
	}
}

func TestTextRelevanceStrongBeatsWeak(t *testing.T) {
	strong := TextRelevanceScore("go", "Go Developer", "Backend Go", "other")
	weak := TextRelevanceScore("go", "Designer", "UI", "once used go briefly")
	if !(strong > weak) {
		t.Fatalf("strong=%v weak=%v", strong, weak)
	}
}
