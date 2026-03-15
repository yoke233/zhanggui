package sqlite

import (
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type ResourceSpaceModel struct {
	ID        int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID int64                     `gorm:"column:project_id;not null;index:idx_resource_spaces_project"`
	Kind      string                    `gorm:"column:kind;not null"`
	RootURI   string                    `gorm:"column:root_uri;not null"`
	Role      string                    `gorm:"column:role;not null;default:''"`
	Label     string                    `gorm:"column:label;not null;default:''"`
	Config    JSONField[map[string]any] `gorm:"column:config;type:text"`
	CreatedAt time.Time                 `gorm:"column:created_at"`
	UpdatedAt time.Time                 `gorm:"column:updated_at"`
}

func (ResourceSpaceModel) TableName() string { return "resource_spaces" }

func resourceSpaceModelFromCore(rs *core.ResourceSpace) *ResourceSpaceModel {
	if rs == nil {
		return nil
	}
	return &ResourceSpaceModel{
		ID:        rs.ID,
		ProjectID: rs.ProjectID,
		Kind:      rs.Kind,
		RootURI:   rs.RootURI,
		Role:      rs.Role,
		Label:     rs.Label,
		Config:    JSONField[map[string]any]{Data: rs.Config},
		CreatedAt: rs.CreatedAt,
		UpdatedAt: rs.UpdatedAt,
	}
}

func (m *ResourceSpaceModel) toCore() *core.ResourceSpace {
	if m == nil {
		return nil
	}
	return &core.ResourceSpace{
		ID:        m.ID,
		ProjectID: m.ProjectID,
		Kind:      m.Kind,
		RootURI:   m.RootURI,
		Role:      m.Role,
		Label:     m.Label,
		Config:    m.Config.Data,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

type ResourceModel struct {
	ID          int64                     `gorm:"column:id;primaryKey;autoIncrement"`
	ProjectID   int64                     `gorm:"column:project_id;not null;index:idx_resources_project"`
	WorkItemID  *int64                    `gorm:"column:work_item_id;index:idx_resources_work_item,where:work_item_id IS NOT NULL"`
	RunID       *int64                    `gorm:"column:run_id;index:idx_resources_run,where:run_id IS NOT NULL"`
	MessageID   *int64                    `gorm:"column:message_id;index:idx_resources_message,where:message_id IS NOT NULL"`
	StorageKind string                    `gorm:"column:storage_kind;not null;default:'local'"`
	URI         string                    `gorm:"column:uri;not null"`
	Role        string                    `gorm:"column:role;not null;default:''"`
	FileName    string                    `gorm:"column:file_name;not null;default:''"`
	MimeType    string                    `gorm:"column:mime_type;not null;default:''"`
	SizeBytes   int64                     `gorm:"column:size_bytes;not null;default:0"`
	Checksum    string                    `gorm:"column:checksum;not null;default:''"`
	Metadata    JSONField[map[string]any] `gorm:"column:metadata;type:text"`
	CreatedAt   time.Time                 `gorm:"column:created_at"`
}

func (ResourceModel) TableName() string { return "resources" }

func resourceModelFromCore(r *core.Resource) *ResourceModel {
	if r == nil {
		return nil
	}
	return &ResourceModel{
		ID:          r.ID,
		ProjectID:   r.ProjectID,
		WorkItemID:  r.WorkItemID,
		RunID:       r.RunID,
		MessageID:   r.MessageID,
		StorageKind: r.StorageKind,
		URI:         r.URI,
		Role:        r.Role,
		FileName:    r.FileName,
		MimeType:    r.MimeType,
		SizeBytes:   r.SizeBytes,
		Checksum:    r.Checksum,
		Metadata:    JSONField[map[string]any]{Data: r.Metadata},
		CreatedAt:   r.CreatedAt,
	}
}

func (m *ResourceModel) toCore() *core.Resource {
	if m == nil {
		return nil
	}
	return &core.Resource{
		ID:          m.ID,
		ProjectID:   m.ProjectID,
		WorkItemID:  m.WorkItemID,
		RunID:       m.RunID,
		MessageID:   m.MessageID,
		StorageKind: m.StorageKind,
		URI:         m.URI,
		Role:        m.Role,
		FileName:    m.FileName,
		MimeType:    m.MimeType,
		SizeBytes:   m.SizeBytes,
		Checksum:    m.Checksum,
		Metadata:    m.Metadata.Data,
		CreatedAt:   m.CreatedAt,
	}
}

type ActionIODeclModel struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ActionID    int64     `gorm:"column:action_id;not null;index:idx_action_io_decls_action,priority:1"`
	Direction   string    `gorm:"column:direction;not null;index:idx_action_io_decls_action,priority:2"`
	SpaceID     *int64    `gorm:"column:space_id"`
	ResourceID  *int64    `gorm:"column:resource_id"`
	Path        string    `gorm:"column:path;not null;default:''"`
	MediaType   string    `gorm:"column:media_type;not null;default:''"`
	Description string    `gorm:"column:description;not null;default:''"`
	Required    bool      `gorm:"column:required;not null;default:false"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (ActionIODeclModel) TableName() string { return "action_io_decls" }

func actionIODeclModelFromCore(decl *core.ActionIODecl) *ActionIODeclModel {
	if decl == nil {
		return nil
	}
	return &ActionIODeclModel{
		ID:          decl.ID,
		ActionID:    decl.ActionID,
		Direction:   string(decl.Direction),
		SpaceID:     decl.SpaceID,
		ResourceID:  decl.ResourceID,
		Path:        decl.Path,
		MediaType:   decl.MediaType,
		Description: decl.Description,
		Required:    decl.Required,
		CreatedAt:   decl.CreatedAt,
	}
}

func (m *ActionIODeclModel) toCore() *core.ActionIODecl {
	if m == nil {
		return nil
	}
	return &core.ActionIODecl{
		ID:          m.ID,
		ActionID:    m.ActionID,
		Direction:   core.IODirection(m.Direction),
		SpaceID:     m.SpaceID,
		ResourceID:  m.ResourceID,
		Path:        m.Path,
		MediaType:   m.MediaType,
		Description: m.Description,
		Required:    m.Required,
		CreatedAt:   m.CreatedAt,
	}
}
