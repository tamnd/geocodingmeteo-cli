package geocodingmeteo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/geocodingmeteo-cli/geocodingmeteo"
)

// twoResultPayload is a canned Open-Meteo search response with 2 locations.
const twoResultPayload = `{
  "results": [
    {
      "id": 2988507,
      "name": "Paris",
      "latitude": 48.85341,
      "longitude": 2.3488,
      "elevation": 42.0,
      "feature_code": "PPLC",
      "country_code": "FR",
      "timezone": "Europe/Paris",
      "population": 2138551,
      "country": "France",
      "admin1": "Île-de-France"
    },
    {
      "id": 5110302,
      "name": "Paris",
      "latitude": 33.66094,
      "longitude": -95.55551,
      "elevation": 148.0,
      "feature_code": "PPL",
      "country_code": "US",
      "timezone": "America/Chicago",
      "population": 25171,
      "country": "United States",
      "admin1": "Texas"
    }
  ]
}`

// newTestClient builds a Client pointed at srv with no rate-limiting.
func newTestClient(srv *httptest.Server) *geocodingmeteo.Client {
	c := geocodingmeteo.NewClient()
	c.BaseURL = srv.URL
	c.Rate = 0
	return c
}

func TestClientGet_UserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestClientGet_RetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearch_TwoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("name"); got != "paris" {
			t.Errorf("name param = %q, want %q", got, "paris")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twoResultPayload))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	locs, err := c.Search(context.Background(), "paris", 10, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 2 {
		t.Fatalf("got %d results, want 2", len(locs))
	}

	if locs[0].ID != 2988507 {
		t.Errorf("locs[0].ID = %d, want 2988507", locs[0].ID)
	}
	if locs[0].Name != "Paris" {
		t.Errorf("locs[0].Name = %q, want Paris", locs[0].Name)
	}
	if locs[0].Country != "France" {
		t.Errorf("locs[0].Country = %q, want France", locs[0].Country)
	}
	if locs[0].Region != "Île-de-France" {
		t.Errorf("locs[0].Region = %q, want Île-de-France", locs[0].Region)
	}
	if locs[1].CountryCode != "US" {
		t.Errorf("locs[1].CountryCode = %q, want US", locs[1].CountryCode)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	locs, err := c.Search(context.Background(), "zzzzunknownplace", 10, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 0 {
		t.Errorf("got %d results, want 0", len(locs))
	}
}

func TestSearch_CountParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("count"); got != "3" {
			t.Errorf("count param = %q, want 3", got)
		}
		resp := map[string]any{"results": []any{}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Search(context.Background(), "london", 3, "en")
	if err != nil {
		t.Fatal(err)
	}
}
