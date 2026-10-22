package handler

import (
	"net/http"
	"strings"

	"logdownloader/internal/auth"
	"logdownloader/internal/model"
	"logdownloader/internal/vlclient"

	"github.com/google/uuid"
)

// datasourceView — API javobi uchun: basic auth paroli client'ga qaytarilmaydi,
// o'rniga `has_password` flag beriladi (UI "saqlangan" placeholder ko'rsatadi).
// Embedded struct'dagi `password` maydonini tashqi (bo'sh, omitempty) maydon bosib ketadi.
type datasourceView struct {
	model.Datasource
	Password    string `json:"password,omitempty"`
	HasPassword bool   `json:"has_password"`
}

func toDatasourceView(ds model.Datasource) datasourceView {
	return datasourceView{Datasource: ds, HasPassword: ds.Password != ""}
}

func (h *Handler) getDatasources(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	list := h.store.GetDatasourcesForUser(u.ID)
	views := make([]datasourceView, 0, len(list))
	for _, ds := range list {
		views = append(views, toDatasourceView(ds))
	}
	writeJSON(w, views)
}

// canUseDatasource — user ko'ra oladigan (o'ziniki yoki global) datasource.
func (h *Handler) canUseDatasource(u *auth.AuthUser, id string) *model.Datasource {
	ds := h.store.GetDatasource(id)
	if ds == nil {
		return nil
	}
	if ds.Global || ds.OwnerID == u.ID || u.Role == model.RoleAdmin {
		return ds
	}
	return nil
}

func normalizeDatasource(ds *model.Datasource) {
	ds.Name = strings.TrimSpace(ds.Name)
	ds.URL = strings.TrimSpace(ds.URL)
	ds.Username = strings.TrimSpace(ds.Username)
	ds.AccountID = strings.TrimSpace(ds.AccountID)
	ds.ProjectID = strings.TrimSpace(ds.ProjectID)
}

func (h *Handler) createDatasource(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	var ds model.Datasource
	if !decode(w, r, &ds) {
		return
	}
	normalizeDatasource(&ds)
	if ds.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	ds.ID = uuid.NewString()
	ds.OwnerID = u.ID
	if ds.Global && u.Role != model.RoleAdmin {
		ds.Global = false
	}

	settings := h.store.GetSettings()
	settings.Datasources = append(settings.Datasources, ds)
	if err := h.store.SaveSettings(settings); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "datasource.create", ds.Name, ds.URL)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, toDatasourceView(ds))
}

func (h *Handler) updateDatasource(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	id := r.PathValue("id")

	var input model.Datasource
	if !decode(w, r, &input) {
		return
	}
	normalizeDatasource(&input)

	settings := h.store.GetSettings()
	found := false
	for i := range settings.Datasources {
		if settings.Datasources[i].ID != id {
			continue
		}
		existing := settings.Datasources[i]
		if existing.OwnerID != u.ID && u.Role != model.RoleAdmin {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		input.ID = existing.ID
		input.OwnerID = existing.OwnerID
		if input.Global && u.Role != model.RoleAdmin {
			input.Global = existing.Global
		}
		// Parol GET javobida qaytarilmaydi: bo'sh parol = "o'zgartirilmasin"
		// (username saqlanib qolgan bo'lsa). Username tozalansa basic auth o'chadi.
		if input.Username == "" {
			input.Password = ""
		} else if input.Password == "" {
			input.Password = existing.Password
		}
		settings.Datasources[i] = input
		found = true
		break
	}
	if !found {
		http.Error(w, "datasource not found", http.StatusNotFound)
		return
	}
	if err := h.store.SaveSettings(settings); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.audit(r, "datasource.update", input.Name, input.URL)
	writeJSON(w, toDatasourceView(input))
}

func (h *Handler) deleteDatasource(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	id := r.PathValue("id")

	settings := h.store.GetSettings()
	var filtered []model.Datasource
	var removed *model.Datasource
	for _, ds := range settings.Datasources {
		if ds.ID == id {
			if ds.OwnerID != u.ID && u.Role != model.RoleAdmin {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			d := ds
			removed = &d
			continue
		}
		filtered = append(filtered, ds)
	}
	settings.Datasources = filtered
	if err := h.store.SaveSettings(settings); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if removed != nil {
		h.audit(r, "datasource.delete", removed.Name, removed.URL)
	}
	w.WriteHeader(http.StatusNoContent)
}

// testDatasource — saqlanmagan (yoki tahrirlanayotgan) datasource bilan
// VictoriaLogs'ga ulanishni tekshirish. `id` berilsa va parol bo'sh bo'lsa
// saqlangan parol ishlatiladi.
func (h *Handler) testDatasource(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	var ds model.Datasource
	if !decode(w, r, &ds) {
		return
	}
	normalizeDatasource(&ds)
	if ds.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	if ds.ID != "" && ds.Password == "" && ds.Username != "" {
		if stored := h.canUseDatasource(u, ds.ID); stored != nil && stored.Username == ds.Username {
			ds.Password = stored.Password
		}
	}
	writeJSON(w, vlclient.TestConnection(r.Context(), ds))
}
