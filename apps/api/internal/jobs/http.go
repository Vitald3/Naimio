package jobs

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"freelance/apps/api/internal/auth"
)

type Handler struct{ Repository Repository }

func (h Handler) Companies(w http.ResponseWriter, r *http.Request) {
	actor, ok := actor(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, e := h.Repository.ListCompanies(r.Context(), actor)
		if domainError(w, r, e) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": v})
	case http.MethodPost:
		var in CompanyInput
		if !decode(w, r, &in) {
			return
		}
		v, e := h.Repository.CreateCompany(r.Context(), actor, in)
		if domainError(w, r, e) {
			return
		}
		write(w, http.StatusCreated, map[string]any{"data": v})
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) PublicCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	cursor, limit, ok := pageInput(w, r)
	if !ok {
		return
	}
	f := Filter{Q: strings.TrimSpace(r.URL.Query().Get("q")), Category: normalizeID(r.URL.Query().Get("category")), Skill: normalizeID(r.URL.Query().Get("skill")), EmploymentType: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("employment_type"))), Location: strings.TrimSpace(r.URL.Query().Get("location")), Experience: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("experience"))), Sort: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("sort")))}
	if raw := r.URL.Query().Get("remote"); raw != "" {
		v, e := strconv.ParseBool(raw)
		if e != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid remote filter")
			return
		}
		f.Remote = &v
	}
	if raw := r.URL.Query().Get("min_salary_kopecks"); raw != "" {
		v, e := strconv.ParseInt(raw, 10, 64)
		if e != nil {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid salary filter")
			return
		}
		f.MinSalary = &v
	}
	v, e := h.Repository.ListPublic(r.Context(), f, cursor, limit)
	if domainError(w, r, e) {
		return
	}
	writePage(w, v)
}

func (h Handler) PublicItem(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/vacancies/"), "/"), "/")
	if len(parts) == 2 && parts[1] == "applications" {
		h.apply(w, r, parts[0])
		return
	}
	if len(parts) != 1 || r.Method != http.MethodGet || len(parts[0]) > 240 || (!uuidPattern.MatchString(parts[0]) && !slugPattern.MatchString(parts[0])) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "vacancy not found")
		return
	}
	v, e := h.Repository.GetPublic(r.Context(), strings.ToLower(parts[0]))
	if domainError(w, r, e) {
		return
	}
	write(w, http.StatusOK, map[string]any{"data": v})
}
func (h Handler) apply(w http.ResponseWriter, r *http.Request, job string) {
	if r.Method != http.MethodPost || !uuidPattern.MatchString(job) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "vacancy not found")
		return
	}
	actor, ok := actor(w, r)
	if !ok {
		return
	}
	var in struct {
		CoverMessage string `json:"cover_message"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, e := h.Repository.Apply(r.Context(), actor, job, in.CoverMessage)
	if domainError(w, r, e) {
		return
	}
	write(w, http.StatusCreated, map[string]any{"data": v})
}

func (h Handler) OwnerCollection(w http.ResponseWriter, r *http.Request) {
	actor, ok := actor(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		c, l, valid := pageInput(w, r)
		if !valid {
			return
		}
		v, e := h.Repository.ListOwned(r.Context(), actor, c, l)
		if domainError(w, r, e) {
			return
		}
		writePage(w, v)
	case http.MethodPost:
		var in CreateRequest
		if !decode(w, r, &in) {
			return
		}
		v, e := h.Repository.Create(r.Context(), actor, in)
		if domainError(w, r, e) {
			return
		}
		write(w, http.StatusCreated, map[string]any{"data": v})
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) OwnerItem(w http.ResponseWriter, r *http.Request) {
	actor, ok := actor(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/me/vacancies/"), "/"), "/")
	if len(parts) < 1 || !uuidPattern.MatchString(parts[0]) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "vacancy not found")
		return
	}
	if len(parts) == 2 && parts[1] == "applications" && r.Method == http.MethodGet {
		v, e := h.Repository.ListApplicants(r.Context(), actor, parts[0])
		if domainError(w, r, e) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": v})
		return
	}
	if len(parts) == 4 && parts[1] == "applications" && parts[3] == "status" && r.Method == http.MethodPost && uuidPattern.MatchString(parts[2]) {
		var in struct {
			Status string `json:"status"`
		}
		if !decode(w, r, &in) {
			return
		}
		v, e := h.Repository.SetApplicationStatus(r.Context(), actor, parts[0], parts[2], in.Status)
		if domainError(w, r, e) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": v})
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost && oneOf(parts[1], "publish", "close") {
		if !empty(w, r) {
			return
		}
		v, e := h.Repository.Transition(r.Context(), actor, parts[0], parts[1])
		if domainError(w, r, e) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": v})
		return
	}
	if len(parts) != 1 {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "vacancy not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, e := h.Repository.GetOwned(r.Context(), actor, parts[0])
		if domainError(w, r, e) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": v})
	case http.MethodPatch:
		var in PatchRequest
		if !decode(w, r, &in) {
			return
		}
		v, e := h.Repository.Update(r.Context(), actor, parts[0], in)
		if domainError(w, r, e) {
			return
		}
		write(w, http.StatusOK, map[string]any{"data": v})
	case http.MethodDelete:
		if domainError(w, r, h.Repository.Delete(r.Context(), actor, parts[0])) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h Handler) Mine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := actor(w, r)
	if !ok {
		return
	}
	c, l, valid := pageInput(w, r)
	if !valid {
		return
	}
	v, e := h.Repository.ListMine(r.Context(), actor, c, l)
	if domainError(w, r, e) {
		return
	}
	write(w, http.StatusOK, map[string]any{"data": v})
}
func (h Handler) Admin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	actor, ok := actor(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/vacancies/"), "/"), "/")
	if len(parts) != 2 || parts[1] != "moderation" || !uuidPattern.MatchString(parts[0]) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "vacancy not found")
		return
	}
	var in struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, e := h.Repository.Moderate(r.Context(), actor, parts[0], in.Action, in.Reason)
	if domainError(w, r, e) {
		return
	}
	write(w, http.StatusOK, map[string]any{"data": v})
}

func actor(w http.ResponseWriter, r *http.Request) (string, bool) {
	v, ok := auth.ActorID(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
	}
	return v, ok
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(target); e != nil {
		decodeError(w, r, e)
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		decodeError(w, r, e)
		return false
	}
	return true
}
func empty(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	b, e := io.ReadAll(r.Body)
	if e != nil {
		decodeError(w, r, e)
		return false
	}
	if v := strings.TrimSpace(string(b)); v != "" && v != "{}" {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "action payload must be empty")
		return false
	}
	return true
}
func pageInput(w http.ResponseWriter, r *http.Request) (*Cursor, int, bool) {
	c, e := decodeCursor(r.URL.Query().Get("cursor"))
	if e != nil {
		writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid cursor")
		return nil, 0, false
	}
	l := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		l, e = strconv.Atoi(raw)
		if e != nil || l < 1 || l > 50 {
			writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid limit")
			return nil, 0, false
		}
	}
	return c, l, true
}

type cursorJSON struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func encodeCursor(c Cursor) string {
	b, _ := json.Marshal(cursorJSON{At: c.At.UTC(), ID: c.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeCursor(v string) (*Cursor, error) {
	if v == "" {
		return nil, nil
	}
	if len(v) > 1024 {
		return nil, ErrInvalidInput
	}
	b, e := base64.RawURLEncoding.DecodeString(v)
	if e != nil {
		return nil, e
	}
	var c cursorJSON
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil || c.At.IsZero() || !uuidPattern.MatchString(c.ID) {
		return nil, ErrInvalidInput
	}
	if e = d.Decode(&struct{}{}); e != io.EOF {
		return nil, ErrInvalidInput
	}
	return &Cursor{At: c.At.UTC(), ID: c.ID}, nil
}
func writePage(w http.ResponseWriter, p Page) {
	var next *string
	if p.NextCursor != nil {
		v := encodeCursor(*p.NextCursor)
		next = &v
	}
	write(w, http.StatusOK, map[string]any{"data": p.Items, "page": map[string]any{"next_cursor": next, "has_more": next != nil}})
}
func domainError(w http.ResponseWriter, r *http.Request, e error) bool {
	if e == nil {
		return false
	}
	switch {
	case errors.Is(e, ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "vacancy not found")
	case errors.Is(e, ErrForbidden):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "operation is not permitted")
	case errors.Is(e, ErrIneligible):
		writeError(w, r, http.StatusForbidden, "FORBIDDEN", "required marketplace capability is missing")
	case errors.Is(e, ErrConflict):
		writeError(w, r, http.StatusConflict, "CONFLICT", "vacancy or application already exists")
	case errors.Is(e, ErrInvalidState):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", "state transition is not allowed")
	case errors.Is(e, ErrInvalidInput):
		message := strings.TrimPrefix(e.Error(), ErrInvalidInput.Error()+": ")
		if message == ErrInvalidInput.Error() || message == "" {
			message = "Проверьте данные вакансии."
		}
		writeError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
	default:
		log.Printf("job operation failure request_id=%s error_type=%T", r.Header.Get("X-Request-ID"), e)
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
	return true
}
func decodeError(w http.ResponseWriter, r *http.Request, e error) {
	var large *http.MaxBytesError
	if errors.As(e, &large) {
		writeError(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "vacancy payload is too large")
		return
	}
	writeError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid vacancy payload")
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	id := w.Header().Get("X-Request-ID")
	if id == "" {
		id = r.Header.Get("X-Request-ID")
	}
	write(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": id}})
}
