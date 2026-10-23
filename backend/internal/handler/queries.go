package handler

import (
	"net/http"
	"strings"

	"logdownloader/internal/auth"
	"logdownloader/internal/model"

	"github.com/google/uuid"
)

// normalizeFolder — bo'sh bo'laklar va chekka slash'larsiz "a/b/c".
func normalizeFolder(f string) string {
	parts := strings.Split(f, "/")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "/")
}

func (h *Handler) getQueries(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	writeJSON(w, h.store.GetQueriesForUser(u.ID))
}

func (h *Handler) createQuery(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	var q model.SavedQuery
	if !decode(w, r, &q) {
		return
	}
	if q.Name == "" || q.Query == "" {
		http.Error(w, "name and query are required", http.StatusBadRequest)
		return
	}
	q.ID = uuid.NewString()
	q.OwnerID = u.ID
	q.Folder = normalizeFolder(q.Folder)
	if q.Global && u.Role != model.RoleAdmin {
		q.Global = false
	}
	if err := h.store.SaveQuery(q); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "query.create", q.Name, "")
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, q)
}

func (h *Handler) updateQuery(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	id := r.PathValue("id")

	var input model.SavedQuery
	if !decode(w, r, &input) {
		return
	}
	var existing *model.SavedQuery
	for _, q := range h.store.GetQueries() {
		if q.ID == id {
			qq := q
			existing = &qq
			break
		}
	}
	if existing == nil {
		http.Error(w, "query not found", http.StatusNotFound)
		return
	}
	if existing.OwnerID != u.ID && u.Role != model.RoleAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	input.ID = existing.ID
	input.OwnerID = existing.OwnerID
	input.Folder = normalizeFolder(input.Folder)
	if input.Global && u.Role != model.RoleAdmin {
		input.Global = existing.Global
	}
	if err := h.store.SaveQuery(input); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, input)
}

func (h *Handler) deleteQuery(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	id := r.PathValue("id")
	for _, q := range h.store.GetQueries() {
		if q.ID == id {
			if q.OwnerID != u.ID && u.Role != model.RoleAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			h.audit(r, "query.delete", q.Name, "")
		}
	}
	if err := h.store.DeleteQuery(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
