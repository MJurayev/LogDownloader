package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"logdownloader/internal/auth"
	"logdownloader/internal/model"
	"logdownloader/internal/vlclient"
)

// dsFromQuery — `datasource_id` parametri bo'yicha foydalanuvchiga ruxsatli datasource.
func (h *Handler) dsFromQuery(w http.ResponseWriter, r *http.Request) *model.Datasource {
	u := auth.GetUser(r)
	dsID := r.URL.Query().Get("datasource_id")
	if dsID == "" {
		http.Error(w, "datasource_id is required", http.StatusBadRequest)
		return nil
	}
	ds := h.canUseDatasource(u, dsID)
	if ds == nil {
		http.Error(w, "datasource not found", http.StatusNotFound)
		return nil
	}
	return ds
}

func (h *Handler) queryLogs(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	ds := h.dsFromQuery(w, r)
	if ds == nil {
		return
	}
	q := r.URL.Query()
	query := q.Get("query")
	if query == "" {
		http.Error(w, "query parameter is required", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	fetchLimit := limit + offset

	params := url.Values{}
	params.Set("query", vlclient.WithTimeSort(query, q.Get("sort")))
	params.Set("limit", strconv.Itoa(fetchLimit))
	if start := q.Get("start"); start != "" {
		params.Set("start", start)
	}
	if end := q.Get("end"); end != "" {
		params.Set("end", end)
	}

	resp, err := vlclient.Do(r.Context(), "GET", vlclient.Endpoint(*ds, "query", params), *ds)
	if err != nil {
		http.Error(w, fmt.Sprintf("VictoriaLogs request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		http.Error(w, fmt.Sprintf("VictoriaLogs error: %s", string(body)), resp.StatusCode)
		return
	}

	var allLogs []json.RawMessage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		allLogs = append(allLogs, json.RawMessage(append([]byte{}, line...)))
	}

	total := len(allLogs)
	hasMore := total == fetchLimit
	if offset >= len(allLogs) {
		allLogs = nil
	} else {
		allLogs = allLogs[offset:]
	}
	if len(allLogs) > limit {
		allLogs = allLogs[:limit]
	}
	if offset == 0 {
		h.store.AddHistory(u.ID, query, ds.ID)
	}
	writeJSON(w, map[string]any{
		"logs":     allLogs,
		"total":    total,
		"has_more": hasMore,
		"limit":    limit,
		"offset":   offset,
	})
}

func queryOrAll(r *http.Request) string {
	q := strings.TrimSpace(r.URL.Query().Get("query"))
	if q == "" {
		return "*"
	}
	return q
}

func vlError(w http.ResponseWriter, err error) {
	http.Error(w, fmt.Sprintf("VictoriaLogs request failed: %v", err), http.StatusBadGateway)
}

func (h *Handler) fieldNames(w http.ResponseWriter, r *http.Request) {
	ds := h.dsFromQuery(w, r)
	if ds == nil {
		return
	}
	q := r.URL.Query()
	values, err := vlclient.FieldNames(r.Context(), *ds, queryOrAll(r), q.Get("start"), q.Get("end"))
	if err != nil {
		vlError(w, err)
		return
	}
	if values == nil {
		values = []vlclient.ValueHits{}
	}
	writeJSON(w, map[string]any{"values": values})
}

func (h *Handler) fieldValues(w http.ResponseWriter, r *http.Request) {
	ds := h.dsFromQuery(w, r)
	if ds == nil {
		return
	}
	q := r.URL.Query()
	field := q.Get("field")
	if field == "" {
		http.Error(w, "field is required", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	values, err := vlclient.FieldValues(r.Context(), *ds, queryOrAll(r), field, limit, q.Get("start"), q.Get("end"))
	if err != nil {
		vlError(w, err)
		return
	}
	if values == nil {
		values = []vlclient.ValueHits{}
	}
	writeJSON(w, map[string]any{"values": values})
}

func (h *Handler) streams(w http.ResponseWriter, r *http.Request) {
	ds := h.dsFromQuery(w, r)
	if ds == nil {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	values, err := vlclient.Streams(r.Context(), *ds, queryOrAll(r), limit, q.Get("start"), q.Get("end"))
	if err != nil {
		vlError(w, err)
		return
	}
	if values == nil {
		values = []vlclient.ValueHits{}
	}
	writeJSON(w, map[string]any{"values": values})
}

// autoStep — gistogramma uchun ~60 bucket beradigan qadam.
func autoStep(start, end string) string {
	s, err1 := time.Parse(time.RFC3339Nano, start)
	e, err2 := time.Parse(time.RFC3339Nano, end)
	if err1 != nil || err2 != nil || !e.After(s) {
		return "1h"
	}
	d := e.Sub(s) / 60
	steps := []time.Duration{time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second, time.Minute,
		5 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour, 3 * time.Hour, 6 * time.Hour,
		12 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour}
	for _, st := range steps {
		if st >= d {
			return fmt.Sprintf("%ds", int64(st.Seconds()))
		}
	}
	return fmt.Sprintf("%ds", int64(steps[len(steps)-1].Seconds()))
}

func (h *Handler) hits(w http.ResponseWriter, r *http.Request) {
	ds := h.dsFromQuery(w, r)
	if ds == nil {
		return
	}
	q := r.URL.Query()
	step := q.Get("step")
	if step == "" {
		step = autoStep(q.Get("start"), q.Get("end"))
	}
	series, err := vlclient.Hits(r.Context(), *ds, queryOrAll(r), q.Get("start"), q.Get("end"), step)
	if err != nil {
		vlError(w, err)
		return
	}
	if series.Timestamps == nil {
		series.Timestamps = []string{}
		series.Values = []int64{}
	}
	writeJSON(w, map[string]any{"timestamps": series.Timestamps, "values": series.Values, "total": series.Total, "step": step})
}

// tail — VictoriaLogs /select/logsql/tail oqimini brauzerga NDJSON sifatida
// uzatadi (chunked, har qatorda flush). Client uzilsa r.Context() orqali
// upstream ham yopiladi.
func (h *Handler) tail(w http.ResponseWriter, r *http.Request) {
	ds := h.dsFromQuery(w, r)
	if ds == nil {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		http.Error(w, "query parameter is required", http.StatusBadRequest)
		return
	}
	params := url.Values{}
	params.Set("query", query)
	resp, err := vlclient.DoLongRunning(r.Context(), "GET", vlclient.Endpoint(*ds, "tail", params), *ds)
	if err != nil {
		vlError(w, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		http.Error(w, fmt.Sprintf("VictoriaLogs error: %s", string(body)), resp.StatusCode)
		return
	}
	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	reader := bufio.NewReaderSize(resp.Body, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func (h *Handler) getHistory(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	list := h.store.GetHistory(u.ID)
	if list == nil {
		list = []model.HistoryEntry{}
	}
	writeJSON(w, list)
}

func (h *Handler) clearHistory(w http.ResponseWriter, r *http.Request) {
	u := auth.GetUser(r)
	h.store.ClearHistory(u.ID)
	w.WriteHeader(http.StatusNoContent)
}
