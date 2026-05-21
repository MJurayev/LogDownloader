package vlclient

import (
	"net"
	"net/http"
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

// longRunningClient — export worker uchun. Hech qanday vaqt limiti yo'q,
// chunki katta export soatlab oqib turishi mumkin. Faqat initial dial va
// TLS handshake'da timeout qoldirilgan (aks holda ulanmagan endpoint'da
// abadiy osilib qoladi).
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

func NewRequest(method, url string, ds model.Datasource) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	if ds.Username != "" {
		req.SetBasicAuth(ds.Username, ds.Password)
	}

	for k, v := range ds.Headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

func Do(method, url string, ds model.Datasource) (*http.Response, error) {
	req, err := NewRequest(method, url, ds)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

// DoLongRunning — VictoriaLogs javobini uzoq muddat (soatlar) stream qilish
// uchun. Export worker'da ishlatiladi.
func DoLongRunning(method, url string, ds model.Datasource) (*http.Response, error) {
	req, err := NewRequest(method, url, ds)
	if err != nil {
		return nil, err
	}
	return longRunningClient.Do(req)
}
