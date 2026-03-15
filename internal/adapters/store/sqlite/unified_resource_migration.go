package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"gorm.io/gorm"
)

func migrateUnifiedResources(ctx context.Context, orm *gorm.DB) error {
	if orm == nil {
		return fmt.Errorf("nil orm")
	}
	if err := migrateLegacyBindings(ctx, orm); err != nil {
		return err
	}
	if err := migrateLegacyActionResources(ctx, orm); err != nil {
		return err
	}
	if err := migrateLegacyRunAssets(ctx, orm); err != nil {
		return err
	}
	return nil
}

func migrateLegacyBindings(ctx context.Context, orm *gorm.DB) error {
	var bindings []ResourceBindingModel
	if err := orm.WithContext(ctx).Order("id ASC").Find(&bindings).Error; err != nil {
		return fmt.Errorf("list legacy resource bindings: %w", err)
	}
	for _, binding := range bindings {
		if strings.EqualFold(strings.TrimSpace(binding.Kind), "attachment") {
			if err := migrateLegacyAttachment(ctx, orm, binding); err != nil {
				return err
			}
			continue
		}
		if err := migrateLegacySpace(ctx, orm, binding); err != nil {
			return err
		}
	}
	return nil
}

func migrateLegacyAttachment(ctx context.Context, orm *gorm.DB, binding ResourceBindingModel) error {
	var count int64
	if err := orm.WithContext(ctx).
		Model(&ResourceModel{}).
		Where("uri = ? AND work_item_id = ?", binding.URI, binding.IssueID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("check migrated attachment %d: %w", binding.ID, err)
	}
	if count > 0 {
		return nil
	}

	projectID := binding.ProjectID
	if projectID == 0 && binding.IssueID != nil {
		var issue WorkItemModel
		if err := orm.WithContext(ctx).First(&issue, *binding.IssueID).Error; err == nil && issue.ProjectID != nil {
			projectID = *issue.ProjectID
		}
	}

	mimeType := ""
	if binding.Config.Data != nil {
		if raw, ok := binding.Config.Data["mime_type"].(string); ok {
			mimeType = raw
		}
	}
	var sizeBytes int64
	if binding.Config.Data != nil {
		switch raw := binding.Config.Data["size"].(type) {
		case float64:
			sizeBytes = int64(raw)
		case int64:
			sizeBytes = raw
		case int:
			sizeBytes = int64(raw)
		}
	}

	model := &ResourceModel{
		ProjectID:   projectID,
		WorkItemID:  binding.IssueID,
		StorageKind: "local",
		URI:         binding.URI,
		Role:        "input",
		FileName:    binding.Label,
		MimeType:    mimeType,
		SizeBytes:   sizeBytes,
		Metadata:    JSONField[map[string]any]{Data: map[string]any{"legacy_binding_id": binding.ID, "legacy_source": "resource_bindings_attachment"}},
	}
	if err := orm.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("migrate attachment binding %d: %w", binding.ID, err)
	}
	return nil
}

func migrateLegacySpace(ctx context.Context, orm *gorm.DB, binding ResourceBindingModel) error {
	var count int64
	if err := orm.WithContext(ctx).
		Model(&ResourceSpaceModel{}).
		Where("project_id = ? AND kind = ? AND root_uri = ?", binding.ProjectID, binding.Kind, binding.URI).
		Count(&count).Error; err != nil {
		return fmt.Errorf("check migrated resource space %d: %w", binding.ID, err)
	}
	if count > 0 {
		return nil
	}

	config := map[string]any{}
	for k, v := range binding.Config.Data {
		config[k] = v
	}
	config["legacy_binding_id"] = binding.ID

	model := &ResourceSpaceModel{
		ID:        binding.ID,
		ProjectID: binding.ProjectID,
		Kind:      binding.Kind,
		RootURI:   binding.URI,
		Label:     binding.Label,
		Config:    JSONField[map[string]any]{Data: config},
	}
	if err := orm.WithContext(ctx).Create(model).Error; err != nil {
		return fmt.Errorf("migrate resource space %d: %w", binding.ID, err)
	}
	return nil
}

func migrateLegacyActionResources(ctx context.Context, orm *gorm.DB) error {
	var decls []ActionResourceModel
	if err := orm.WithContext(ctx).Order("id ASC").Find(&decls).Error; err != nil {
		return fmt.Errorf("list legacy action resources: %w", err)
	}
	for _, decl := range decls {
		var binding ResourceBindingModel
		if err := orm.WithContext(ctx).First(&binding, decl.ResourceBindingID).Error; err != nil {
			continue
		}

		newDecl := &ActionIODeclModel{
			ActionID:    decl.ActionID,
			Direction:   decl.Direction,
			Path:        decl.Path,
			MediaType:   decl.MediaType,
			Description: decl.Description,
			Required:    decl.Required,
		}
		if strings.EqualFold(strings.TrimSpace(binding.Kind), "attachment") {
			var resource ResourceModel
			if err := orm.WithContext(ctx).
				Where("uri = ? AND work_item_id = ?", binding.URI, binding.IssueID).
				First(&resource).Error; err != nil {
				continue
			}
			newDecl.ResourceID = &resource.ID
		} else {
			var space ResourceSpaceModel
			if err := orm.WithContext(ctx).
				Where("project_id = ? AND kind = ? AND root_uri = ?", binding.ProjectID, binding.Kind, binding.URI).
				First(&space).Error; err != nil {
				continue
			}
			newDecl.SpaceID = &space.ID
		}

		var count int64
		query := orm.WithContext(ctx).Model(&ActionIODeclModel{}).
			Where("action_id = ? AND direction = ? AND path = ? AND media_type = ? AND description = ? AND required = ?",
				newDecl.ActionID, newDecl.Direction, newDecl.Path, newDecl.MediaType, newDecl.Description, newDecl.Required)
		if newDecl.SpaceID != nil {
			query = query.Where("space_id = ? AND resource_id IS NULL", *newDecl.SpaceID)
		} else {
			query = query.Where("resource_id = ? AND space_id IS NULL", *newDecl.ResourceID)
		}
		if err := query.Count(&count).Error; err != nil {
			return fmt.Errorf("check migrated action io decl %d: %w", decl.ID, err)
		}
		if count > 0 {
			continue
		}
		if err := orm.WithContext(ctx).Create(newDecl).Error; err != nil {
			return fmt.Errorf("migrate action resource %d: %w", decl.ID, err)
		}
	}
	return nil
}

func migrateLegacyRunAssets(ctx context.Context, orm *gorm.DB) error {
	var runs []RunModel
	if err := orm.WithContext(ctx).
		Where("result_assets IS NOT NULL").
		Order("id ASC").
		Find(&runs).Error; err != nil {
		return fmt.Errorf("list legacy run assets: %w", err)
	}
	for _, run := range runs {
		if len(run.ResultAssets.Data) == 0 {
			continue
		}
		projectID := int64(0)
		var issue WorkItemModel
		if err := orm.WithContext(ctx).First(&issue, run.IssueID).Error; err == nil && issue.ProjectID != nil {
			projectID = *issue.ProjectID
		}
		for _, asset := range run.ResultAssets.Data {
			var count int64
			if err := orm.WithContext(ctx).
				Model(&ResourceModel{}).
				Where("run_id = ? AND uri = ? AND file_name = ?", run.ID, asset.URI, asset.Name).
				Count(&count).Error; err != nil {
				return fmt.Errorf("check migrated run asset %d: %w", run.ID, err)
			}
			if count > 0 {
				continue
			}
			storageKind := detectStorageKind(asset.URI)
			model := &ResourceModel{
				ProjectID:   projectID,
				RunID:       &run.ID,
				StorageKind: storageKind,
				URI:         asset.URI,
				Role:        "output",
				FileName:    asset.Name,
				MimeType:    asset.MediaType,
				Metadata:    JSONField[map[string]any]{Data: map[string]any{"legacy_source": "executions.result_assets"}},
			}
			if err := orm.WithContext(ctx).Create(model).Error; err != nil {
				return fmt.Errorf("migrate run asset %d/%s: %w", run.ID, asset.Name, err)
			}
		}
	}
	return nil
}

func detectStorageKind(uri string) string {
	trimmed := strings.TrimSpace(strings.ToLower(uri))
	switch {
	case strings.HasPrefix(trimmed, "s3://"):
		return "s3"
	case strings.HasPrefix(trimmed, "http://"), strings.HasPrefix(trimmed, "https://"):
		return "http"
	case strings.HasPrefix(trimmed, "file://"):
		return "local"
	case filepath.IsAbs(uri):
		return "local"
	default:
		return "local"
	}
}
