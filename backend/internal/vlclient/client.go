package vlclient

import (
	"net/http"

	"logdownloader/internal/model"
)

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
