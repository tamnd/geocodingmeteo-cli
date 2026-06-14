package geocodingmeteo

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring, which need no network. The client's HTTP behaviour is
// covered in geocodingmeteo_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "geocodingmeteo" {
		t.Errorf("Scheme = %q, want geocodingmeteo", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "geocodingmeteo" {
		t.Errorf("Identity.Binary = %q, want geocodingmeteo", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in      string
		wantTyp string
		wantID  string
	}{
		{"paris", "query", "paris"},
		{"New York", "query", "New York"},
		{"2988507", "location", "2988507"},
		{"0", "location", "0"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil {
			t.Errorf("Classify(%q) returned error: %v", tc.in, err)
			continue
		}
		if typ != tc.wantTyp || id != tc.wantID {
			t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)",
				tc.in, typ, id, tc.wantTyp, tc.wantID)
		}
	}
}

func TestClassify_EmptyError(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify(\"\") should return an error")
	}
}

func TestLocate(t *testing.T) {
	base := DefaultConfig().BaseURL

	cases := []struct {
		uriType string
		id      string
		want    string
	}{
		{"location", "2988507", base + "/v1/search?name=2988507&count=1"},
		{"query", "paris", base + "/v1/search?name=paris"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if err != nil {
			t.Errorf("Locate(%q, %q) error: %v", tc.uriType, tc.id, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Locate(%q, %q) = %q, want %q", tc.uriType, tc.id, got, tc.want)
		}
	}
}

func TestLocate_UnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "foo")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round trip:
// a record mints to its URI, and the domain resolves bare ids correctly.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	loc := &Location{
		ID:      2988507,
		Name:    "Paris",
		Country: "France",
	}
	u, err := h.Mint(loc)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "geocodingmeteo://location/2988507"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("geocodingmeteo", "2988507")
	if err != nil || got.String() != "geocodingmeteo://location/2988507" {
		t.Errorf("ResolveOn = (%q, %v), want geocodingmeteo://location/2988507", got.String(), err)
	}
}
