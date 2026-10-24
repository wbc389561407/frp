package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzNoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if user, pass, ok := r.BasicAuth(); ok {
			t.Errorf("healthz should be anonymous, got %q/%q", user, pass)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := Client{base: srv.URL, client: srv.Client()}
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatalf("healthz: %v", err)
	}
}

func TestStatusParsesSnakeCase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"tcp":[{"name":"ssh","type":"tcp","status":"running","err":"","local_addr":"127.0.0.1:22","plugin":"","remote_addr":"1.2.3.4:6002","source":"store"}]}`))
	}))
	defer srv.Close()

	c := New(0, "admin", "secret")
	c.base = srv.URL
	res, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	ps := res["tcp"][0]
	if ps.Name != "ssh" || ps.Status != "running" || ps.RemoteAddr != "1.2.3.4:6002" || ps.Source != "store" {
		t.Fatalf("unexpected status: %+v", ps)
	}
}

func TestErrorBodyCapitalizedKeys(t *testing.T) {
	// frpc 错误体为 {"Code":..,"Msg":..}（首字母大写）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"Code":409,"Msg":"proxy already exists"}`))
	}))
	defer srv.Close()

	c := Client{base: srv.URL, client: srv.Client()}
	_, err := c.do(context.Background(), http.MethodPost, "/api/store/proxies", "{}")
	if err == nil || err.Error() != "api /api/store/proxies: 409 proxy already exists" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := Client{base: srv.URL, user: "admin", pwd: "bad", client: srv.Client()}
	if err := c.Stop(context.Background()); err == nil {
		t.Fatal("expected error on 401")
	}
}
