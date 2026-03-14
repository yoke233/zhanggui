package configruntime

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
)

type RegistrySyncStore interface {
	ListProfiles(ctx context.Context) ([]*core.AgentProfile, error)
	UpsertProfile(ctx context.Context, p *core.AgentProfile) error
	DeleteProfile(ctx context.Context, id string) error
}

func SyncRegistry(ctx context.Context, store RegistrySyncStore, snap *Snapshot) error {
	if store == nil || snap == nil {
		return nil
	}
	currentProfiles, err := store.ListProfiles(ctx)
	if err != nil {
		return fmt.Errorf("list profiles for sync: %w", err)
	}

	wantedProfiles := make(map[string]struct{}, len(snap.Profiles))
	for _, profile := range snap.Profiles {
		wantedProfiles[profile.ID] = struct{}{}
		if err := store.UpsertProfile(ctx, profile); err != nil {
			return fmt.Errorf("upsert profile %s: %w", profile.ID, err)
		}
	}

	for _, profile := range currentProfiles {
		if _, ok := wantedProfiles[profile.ID]; ok {
			continue
		}
		if err := store.DeleteProfile(ctx, profile.ID); err != nil {
			return fmt.Errorf("delete stale profile %s: %w", profile.ID, err)
		}
	}

	return nil
}
