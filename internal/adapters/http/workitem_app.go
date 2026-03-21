package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/yoke233/zhanggui/internal/application/workitemapp"
	"github.com/yoke233/zhanggui/internal/core"
)

type workItemAppBootstrapper struct {
	handler *Handler
}

func (b workItemAppBootstrapper) BootstrapPRWorkItem(ctx context.Context, workItemID int64) error {
	if b.handler == nil {
		return nil
	}
	_, err := b.handler.bootstrapPRWorkItemActions(ctx, workItemID, bootstrapPRWorkItemRequest{})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errBootstrapPRWorkItemMissingProject),
		errors.Is(err, errBootstrapPRWorkItemMissingSpace),
		errors.Is(err, errBootstrapPRWorkItemAmbiguousSpace):
		return nil
	default:
		return err
	}
}

func (h *Handler) workItemService() *workitemapp.Service {
	if h == nil {
		return nil
	}
	var tx workitemapp.Tx
	if txStore, ok := h.store.(core.TransactionalStore); ok {
		tx = workItemAppTx{store: txStore}
	}
	return workitemapp.New(workitemapp.Config{
		Store:       h.store,
		Tx:          tx,
		Scheduler:   h.scheduler,
		Runner:      h.engine,
		Bus:         h.bus,
		BootstrapPR: workItemAppBootstrapper{handler: h},
	})
}

type workItemAppTx struct {
	store core.TransactionalStore
}

func (t workItemAppTx) InTx(ctx context.Context, fn func(ctx context.Context, store workitemapp.TxStore) error) error {
	if t.store == nil {
		return fmt.Errorf("work item transaction adapter is not configured")
	}
	return t.store.InTx(ctx, func(store core.Store) error {
		txStore, ok := store.(workitemapp.TxStore)
		if !ok {
			return fmt.Errorf("transaction store %T does not implement workitemapp tx store", store)
		}
		return fn(ctx, txStore)
	})
}

func writeWorkItemAppError(w http.ResponseWriter, err error) bool {
	switch workitemapp.CodeOf(err) {
	case workitemapp.CodeWorkItemNotFound:
		writeError(w, http.StatusNotFound, "work item not found", "NOT_FOUND")
	case workitemapp.CodeProjectNotFound:
		writeError(w, http.StatusNotFound, "project not found", "PROJECT_NOT_FOUND")
	case workitemapp.CodeResourceBindingNotFound:
		writeError(w, http.StatusNotFound, "resource binding not found", "RESOURCE_BINDING_NOT_FOUND")
	case workitemapp.CodeWorkItemDependencyNotFound:
		writeError(w, http.StatusNotFound, "dependency work item not found", "WORK_ITEM_DEPENDENCY_NOT_FOUND")
	case workitemapp.CodeMissingTitle:
		writeError(w, http.StatusBadRequest, "title is required", "MISSING_TITLE")
	case workitemapp.CodeInvalidResourceBinding:
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_RESOURCE_BINDING")
	case workitemapp.CodeInvalidWorkItemDependency:
		writeError(w, http.StatusBadRequest, err.Error(), "INVALID_WORK_ITEM_DEPENDENCY")
	case workitemapp.CodeNoActions:
		writeError(w, http.StatusBadRequest, "work item has no actions; add at least one action before running", "NO_ACTIONS")
	case workitemapp.CodeInvalidState:
		writeError(w, http.StatusConflict, err.Error(), "INVALID_STATE")
	case workitemapp.CodeBootstrapPRFailed:
		writeError(w, http.StatusInternalServerError, err.Error(), "AUTO_SCM_WORK_ITEM_BOOTSTRAP_FAILED")
	default:
		return false
	}
	return true
}

func writeWorkItemAppFailure(w http.ResponseWriter, err error, fallbackCode string) {
	if writeWorkItemAppError(w, err) {
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error(), fallbackCode)
}
