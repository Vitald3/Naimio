package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"freelance/apps/api/internal/media"
)

type StorageHandler struct {
	Service Service
	ActorID func(http.ResponseWriter, *http.Request) (string, bool)
	Manager *media.StorageManager
}

func (h StorageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.ActorID(w, r)
	if !ok {
		return
	}
	if err := h.Service.require(r.Context(), actor, "ADMIN"); err != nil {
		handle(w, r, err)
		return
	}

	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/storage-settings"), "/")

	if path == "test" {
		if r.Method != http.MethodPost {
			method(w, r)
			return
		}
		var cfg media.S3UpdateConfig
		if !decode(w, r, &cfg) {
			return
		}
		if h.Manager == nil {
			problem(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "storage manager is not initialized")
			return
		}
		if err := h.Manager.TestConnection(r.Context(), cfg); err != nil {
			problem(w, r, http.StatusUnprocessableEntity, "CONNECTION_FAILED", err.Error())
			return
		}
		reply(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"success": true,
				"message": "Подключение успешно",
			},
		})
		return
	}

	if path != "" {
		notFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if h.Manager == nil {
			problem(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "storage manager is not initialized")
			return
		}
		settings, err := h.Manager.GetSettings(r.Context())
		if err != nil {
			problem(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load storage settings")
			return
		}
		reply(w, http.StatusOK, map[string]any{"data": settings})
	case http.MethodPut, http.MethodPatch:
		var update media.StorageSettingsUpdate
		if !decode(w, r, &update) {
			return
		}
		if h.Manager == nil {
			problem(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "storage manager is not initialized")
			return
		}
		saved, err := h.Manager.UpdateSettings(r.Context(), actor, update)
		if err != nil {
			problem(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", err.Error())
			return
		}
		reply(w, http.StatusOK, map[string]any{"data": saved})
	default:
		method(w, r)
	}
}

// AttachStorageHandler creates a StorageHandler using the admin Handler's actor resolver
func (h Handler) StorageSettingsHandler(manager *media.StorageManager) http.Handler {
	return StorageHandler{
		Service: h.Service,
		ActorID: func(w http.ResponseWriter, r *http.Request) (string, bool) {
			return h.actor(w, r)
		},
		Manager: manager,
	}
}

// Unused dummy to ensure json import is active
var _ = json.Marshal
