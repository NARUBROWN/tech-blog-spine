package blog

import "testing"

func TestSlugifyMatchesSpringContract(t *testing.T) {
	if got := Slugify("  Spine   마이그레이션 가이드  "); got != "Spine-마이그레이션-가이드" {
		t.Fatalf("slug = %q", got)
	}
}

func TestNormalizePageBounds(t *testing.T) {
	page, size := normalizePage(-1, 0)
	if page != 0 || size != 20 {
		t.Fatalf("기본 페이지 = %d, %d", page, size)
	}
	_, size = normalizePage(1, 1000)
	if size != 100 {
		t.Fatalf("최대 페이지 크기 = %d", size)
	}
}
