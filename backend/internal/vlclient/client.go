package vlclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"logdownloader/internal/model"
)

// WithTimeSort foydalanuvchining LogsQL query'siga `| sort by (_time)` pipe
// qo'shadi. Agar query'da `|` mavjud bo'lsa (user o'z pipe'lari bilan) yoki
// order = "" bo'lsa, query o'zgarmaydi. "desc" bo'lsa kamayish tartibida.
func WithTimeSort(query, order string) string {
	if order == "" {
		return query
	}
	if strings.Contains(query, "|") {
		return query
	}
	if order == "desc" {
		return query + " | sort by (_time) desc"
	}
	return query + " | sort by (_time)"
}

// HasPipes — query'da pipe bor-yo'qligi (hits/field_names kabi endpointlar
// faqat filtr qismini kutadi; pipe'li query'da ular chaqirilmaydi).
func HasPipes(query string) bool { return strings.Contains(query, "|") }

// longRunningClient — export worker uchun. Hech qanday vaqt limiti yo'q,
// chunki katta export soatlab oqib turishi mumkin. Faqat initial dial va
// TLS handshake'da timeout qoldirilgan. Bekor qilish faqat context orqali.
var longRunningClient = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       0,
		TLSHandshakeTimeout:   30 * time.Second,
		ExpectContinueTimeout: 0,
		ResponseHeaderTimeout: 0,
	},
}

// shortClient — qisqa so'rovlar (viewer, hits, field_names, test) uchun.
var shortClient = &http.Client{Timeout: 60 * time.Second}

// BaseURL — datasource URL'ining oxiridagi slash'siz ko'rinishi.
func BaseURL(ds model.Datasource) string { return strings.TrimRight(ds.URL, "/") }

// Endpoint — `<base>/select/logsql/<name>?<params>`.
func Endpoint(ds model.Datasource, name string, params url.Values) string {
	u := fmt.Sprintf("%s/select/logsql/%s", BaseURL(ds), name)
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	return u
}

func NewRequest(ctx context.Context, method, url string, ds model.Datasource) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	if ds.Username != "" {
		req.SetBasicAuth(ds.Username, ds.Password)
	}
	for k, v := range ds.Headers {
		req.Header.Set(k, v)
	}
	// VictoriaLogs multitenancy (ixtiyoriy).
	if ds.AccountID != "" {
		req.Header.Set("AccountID", ds.AccountID)
	}
	if ds.ProjectID != "" {
		req.Header.Set("ProjectID", ds.ProjectID)
	}
	return req, nil
}

// Do — qisqa so'rovlar uchun (60s timeout).
func Do(ctx context.Context, method, url string, ds model.Datasource) (*http.Response, error) {
	req, err := NewRequest(ctx, method, url, ds)
	if err != nil {
		return nil, err
	}
	return shortClient.Do(req)
}

// DoLongRunning — VictoriaLogs javobini uzoq muddat (soatlar) stream qilish
// uchun. Export worker va tail'da ishlatiladi; ctx bekor qilinsa oqim uziladi.
func DoLongRunning(ctx context.Context, method, url string, ds model.Datasource) (*http.Response, error) {
	req, err := NewRequest(ctx, method, url, ds)
	if err != nil {
		return nil, err
	}
	return longRunningClient.Do(req)
}

// getJSON — GET + JSON decode; 2xx bo'lmasa javob matni bilan xato.
func getJSON(ctx context.Context, ds model.Datasource, name string, params url.Values, out any) error {
	resp, err := Do(ctx, "GET", Endpoint(ds, name, params), ds)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("VictoriaLogs HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ValueHits — field_names / field_values / streams javobidagi element.
type ValueHits struct {
	Value string `json:"value"`
	Hits  int64  `json:"hits"`
}

type valuesResp struct {
	Values []ValueHits `json:"values"`
}

func timeParams(query, start, end string) url.Values {
	p := url.Values{}
	p.Set("query", query)
	if start != "" {
		p.Set("start", start)
	}
	if end != "" {
		p.Set("end", end)
	}
	return p
}

func FieldNames(ctx context.Context, ds model.Datasource, query, start, end string) ([]ValueHits, error) {
	var r valuesResp
	if err := getJSON(ctx, ds, "field_names", timeParams(query, start, end), &r); err != nil {
		return nil, err
	}
	return r.Values, nil
}

func FieldValues(ctx context.Context, ds model.Datasource, query, field string, limit int, start, end string) ([]ValueHits, error) {
	p := timeParams(query, start, end)
	p.Set("field", field)
	if limit > 0 {
		p.Set("limit", fmt.Sprint(limit))
	}
	var r valuesResp
	if err := getJSON(ctx, ds, "field_values", p, &r); err != nil {
		return nil, err
	}
	return r.Values, nil
}

func Streams(ctx context.Context, ds model.Datasource, query string, limit int, start, end string) ([]ValueHits, error) {
	p := timeParams(query, start, end)
	if limit > 0 {
		p.Set("limit", fmt.Sprint(limit))
	}
	var r valuesResp
	if err := getJSON(ctx, ds, "streams", p, &r); err != nil {
		return nil, err
	}
	return r.Values, nil
}

// HitsSeries — /select/logsql/hits javobi (bitta seriya, field'siz).
type HitsSeries struct {
	Timestamps []string `json:"timestamps"`
	Values     []int64  `json:"values"`
	Total      int64    `json:"total"`
}

type hitsResp struct {
	Hits []HitsSeries `json:"hits"`
}

// Hits — vaqt bo'yicha gistogramma. step: masalan "5m", "1h".
func Hits(ctx context.Context, ds model.Datasource, query, start, end, step string) (HitsSeries, error) {
	p := timeParams(query, start, end)
	if step != "" {
		p.Set("step", step)
	}
	var r hitsResp
	if err := getJSON(ctx, ds, "hits", p, &r); err != nil {
		return HitsSeries{}, err
	}
	if len(r.Hits) == 0 {
		return HitsSeries{}, nil
	}
	// field'siz so'rovda bitta seriya; ehtiyot uchun hammasini jamlaymiz.
	out := r.Hits[0]
	for _, s := range r.Hits[1:] {
		out.Total += s.Total
		for i := range s.Values {
			if i < len(out.Values) {
				out.Values[i] += s.Values[i]
			}
		}
	}
	return out, nil
}

// CountHits — oraliqdagi mos qatorlar soni (export progress uchun taxmin).
// Pipe'li query'da ishlatilmaydi (caller tekshiradi).
func CountHits(ctx context.Context, ds model.Datasource, query, start, end string, rangeDur time.Duration) (int64, error) {
	step := "1y"
	if rangeDur > 0 {
		// Bitta bucket: step = oraliq (soniyalarda). VictoriaLogs step'ni duration sifatida oladi.
		step = fmt.Sprintf("%ds", int64(rangeDur.Seconds())+1)
	}
	s, err := Hits(ctx, ds, query, start, end, step)
	if err != nil {
		return 0, err
	}
	if s.Total > 0 {
		return s.Total, nil
	}
	var sum int64
	for _, v := range s.Values {
		sum += v
	}
	return sum, nil
}

// EarliestTime — query'ga mos eng eski logning _time qiymati (ochiq oraliqni
// oynalarga bo'lish uchun). Pipe'li query'da xato qaytaradi.
func EarliestTime(ctx context.Context, ds model.Datasource, query, end string) (time.Time, error) {
	if HasPipes(query) {
		return time.Time{}, fmt.Errorf("query has pipes")
	}
	p := url.Values{}
	p.Set("query", query+" | sort by (_time) | limit 1")
	p.Set("limit", "1")
	if end != "" {
		p.Set("end", end)
	}
	resp, err := Do(ctx, "GET", Endpoint(ds, "query", p), ds)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return time.Time{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return time.Time{}, err
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return time.Time{}, fmt.Errorf("no logs")
	}
	if i := strings.IndexByte(line, '\n'); i > 0 {
		line = line[:i]
	}
	var row struct {
		Time string `json:"_time"`
	}
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, row.Time)
}

// TestResult — datasource ulanish tekshiruvi natijasi.
type TestResult struct {
	OK        bool   `json:"ok"`
	Status    int    `json:"status,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// TestConnection — `/select/logsql/query?query=*&limit=1` ga 10s timeout bilan.
// Auth/tenant header'lari ham tekshiriladi.
func TestConnection(ctx context.Context, ds model.Datasource) TestResult {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p := url.Values{}
	p.Set("query", "*")
	p.Set("limit", "1")
	t0 := time.Now()
	resp, err := Do(ctx, "GET", Endpoint(ds, "query", p), ds)
	res := TestResult{LatencyMs: time.Since(t0).Milliseconds()}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	res.Status = resp.StatusCode
	res.LatencyMs = time.Since(t0).Milliseconds()
	if resp.StatusCode == http.StatusOK {
		res.OK = true
	} else {
		res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return res
}
