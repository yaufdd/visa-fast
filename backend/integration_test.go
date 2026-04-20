//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/server"
)

func startTestServer(t *testing.T, pool *pgxpool.Pool) *httptest.Server {
	t.Helper()
	handler := server.NewRouter(pool, "dummy-ai-key", t.TempDir(), "/tmp/generate.py")
	return httptest.NewServer(handler)
}

func registerOrg(t *testing.T, srv *httptest.Server, orgName, email, password string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{
		"org_name": orgName, "email": email, "password": password,
	})
	resp, err := client.Post(srv.URL+"/api/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("register %s: %d — %s", orgName, resp.StatusCode, b)
	}
	return client
}

func TestCrossOrgIsolation(t *testing.T) {
	if os.Getenv("POSTGRES_HOST") == "" {
		t.Skip("integration test requires POSTGRES_HOST env")
	}
	dsn := fmt.Sprintf("postgres://fuji:fuji123@%s:%s/fujitravel?sslmode=disable",
		os.Getenv("POSTGRES_HOST"), os.Getenv("POSTGRES_PORT"))
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	srv := startTestServer(t, pool)
	defer srv.Close()

	c1 := registerOrg(t, srv, "Org One", "o1@test.com", "password123")
	c2 := registerOrg(t, srv, "Org Two", "o2@test.com", "password123")

	// Org1 creates a group
	body, _ := json.Marshal(map[string]string{"name": "SecretGroup"})
	resp, err := c1.Post(srv.URL+"/api/groups", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create group: %d — %s", resp.StatusCode, b)
	}
	var g struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&g)
	resp.Body.Close()

	// Org2 list → empty
	resp, _ = c2.Get(srv.URL + "/api/groups")
	var groups []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&groups)
	resp.Body.Close()
	if len(groups) != 0 {
		t.Errorf("org2 sees org1's groups: %v", groups)
	}

	// Org2 direct access → 404
	resp, _ = c2.Get(srv.URL + "/api/groups/" + g.ID)
	if resp.StatusCode != 404 {
		t.Errorf("org2 GET org1 group: %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	// Org2 delete → 404
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/groups/"+g.ID, nil)
	resp, _ = c2.Do(req)
	if resp.StatusCode != 404 {
		t.Errorf("org2 DELETE org1 group: %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	// Org1 still sees it
	resp, _ = c1.Get(srv.URL + "/api/groups/" + g.ID)
	if resp.StatusCode != 200 {
		t.Errorf("org1 GET own group: %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}
