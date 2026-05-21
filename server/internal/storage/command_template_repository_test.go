package storage_test

import (
	"context"
	"testing"

	"hostdeck/server/internal/domain"
	"hostdeck/server/internal/storage"
)

func TestCommandTemplateRepository_CreateListAndFavorite(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewCommandTemplateRepository(db)
	if err := repo.EnsureDefaults(context.Background(), []domain.CommandTemplate{{
		ID:          "shared-disk-usage",
		Name:        "磁盘使用率",
		Description: "检查磁盘",
		Command:     "df -h",
		Scope:       domain.CommandTemplateScopeShared,
		RiskLevel:   domain.CommandTemplateRiskNormal,
	}}); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}

	created, err := repo.Create(context.Background(), domain.CommandTemplateCreateInput{
		Name:        "检查 nginx 状态",
		Description: "巡检 nginx",
		Command:     "systemctl status {{service}} --no-pager",
		Scope:       domain.CommandTemplateScopePersonal,
		RiskLevel:   domain.CommandTemplateRiskNormal,
		Variables: []domain.CommandTemplateVariable{{
			Name:     "service",
			Label:    "服务名",
			Required: true,
		}},
	}, "operator")
	if err != nil {
		t.Fatalf("create personal template: %v", err)
	}
	if created.Scope != domain.CommandTemplateScopePersonal || created.CreatedBy != "operator" {
		t.Fatalf("unexpected created template: %+v", created)
	}
	if !created.IsFavorite {
		t.Fatal("expected personal template to default favorite")
	}

	operatorItems, err := repo.List(context.Background(), domain.CommandTemplateFilter{Username: "operator"})
	if err != nil {
		t.Fatalf("list operator templates: %v", err)
	}
	if len(operatorItems) != 2 {
		t.Fatalf("expected 2 templates for operator, got %d", len(operatorItems))
	}
	if operatorItems[0].ID != created.ID || !operatorItems[0].IsFavorite {
		t.Fatalf("expected personal favorite template first, got %+v", operatorItems)
	}

	viewerItems, err := repo.List(context.Background(), domain.CommandTemplateFilter{Username: "viewer"})
	if err != nil {
		t.Fatalf("list viewer templates: %v", err)
	}
	if len(viewerItems) != 1 || viewerItems[0].Scope != domain.CommandTemplateScopeShared {
		t.Fatalf("expected viewer to only see shared templates, got %+v", viewerItems)
	}

	if err := repo.SetFavorite(context.Background(), "shared-disk-usage", "operator", true); err != nil {
		t.Fatalf("favorite shared template: %v", err)
	}
	operatorItems, err = repo.List(context.Background(), domain.CommandTemplateFilter{Username: "operator"})
	if err != nil {
		t.Fatalf("list operator templates after favorite: %v", err)
	}
	if !operatorItems[0].IsFavorite {
		t.Fatalf("expected first template to be favorite, got %+v", operatorItems[0])
	}

	if err := repo.SetFavorite(context.Background(), "shared-disk-usage", "operator", false); err != nil {
		t.Fatalf("unfavorite shared template: %v", err)
	}
	operatorItems, err = repo.List(context.Background(), domain.CommandTemplateFilter{Username: "operator"})
	if err != nil {
		t.Fatalf("list operator templates after unfavorite: %v", err)
	}
	for _, item := range operatorItems {
		if item.ID == "shared-disk-usage" && item.IsFavorite {
			t.Fatalf("expected shared template favorite to be removed, got %+v", item)
		}
	}
}

func TestCommandTemplateRepository_RejectsInvalidScope(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewCommandTemplateRepository(db)

	_, err := repo.Create(context.Background(), domain.CommandTemplateCreateInput{
		Name:      "非法模板",
		Command:   "echo test",
		Scope:     "invalid",
		RiskLevel: domain.CommandTemplateRiskNormal,
	}, "operator")
	if err == nil {
		t.Fatal("expected invalid scope error")
	}
}

func TestCommandTemplateRepository_GeneratesDistinctIDsForChineseNames(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewCommandTemplateRepository(db)

	first, err := repo.Create(context.Background(), domain.CommandTemplateCreateInput{
		Name:      "检查服务",
		Command:   "echo 1",
		Scope:     domain.CommandTemplateScopePersonal,
		RiskLevel: domain.CommandTemplateRiskNormal,
	}, "运维甲")
	if err != nil {
		t.Fatalf("create first chinese template: %v", err)
	}
	second, err := repo.Create(context.Background(), domain.CommandTemplateCreateInput{
		Name:      "检查磁盘",
		Command:   "echo 2",
		Scope:     domain.CommandTemplateScopePersonal,
		RiskLevel: domain.CommandTemplateRiskNormal,
	}, "运维甲")
	if err != nil {
		t.Fatalf("create second chinese template: %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("expected distinct ids, got %q and %q", first.ID, second.ID)
	}
}

func TestCommandTemplateRepository_SetFavoriteRejectsInvisibleTemplate(t *testing.T) {
	db := openTestDB(t)
	repo := storage.NewCommandTemplateRepository(db)

	created, err := repo.Create(context.Background(), domain.CommandTemplateCreateInput{
		Name:      "我的模板",
		Command:   "echo secret",
		Scope:     domain.CommandTemplateScopePersonal,
		RiskLevel: domain.CommandTemplateRiskNormal,
	}, "operator")
	if err != nil {
		t.Fatalf("create personal template: %v", err)
	}
	if err := repo.SetFavorite(context.Background(), created.ID, "viewer", true); err == nil {
		t.Fatal("expected invisible template favorite to fail")
	} else if err != storage.ErrCommandTemplateAccessDenied {
		t.Fatalf("expected access denied, got %v", err)
	}
}
