package core

import (
	"net/http"
	"net/url"
	"os"
	"time"
)

func NewClient() *http.Client {
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("http_proxy")
	os.Unsetenv("https_proxy")
	os.Unsetenv("NO_PROXY")
	os.Unsetenv("no_proxy")

	transport := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return nil, nil
		},
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	}

	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}
}

func NewRequest(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "luffy/1.0")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	return req, nil
}
