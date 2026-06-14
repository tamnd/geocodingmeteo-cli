package geocodingmeteo

import (
	"context"
	"unicode"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes geocodingmeteo as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/geocodingmeteo-cli/geocodingmeteo"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// geocodingmeteo:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone geocodingmeteo binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the geocodingmeteo driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "geocodingmeteo",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "geocodingmeteo",
			Short:  "Search cities and locations via the Open-Meteo Geocoding API.",
			Long: `Search cities and locations via the Open-Meteo Geocoding API.

geocodingmeteo reads public Open-Meteo geocoding data over plain HTTPS, shapes
it into clean records, and prints output that pipes into the rest of your tools.
No API key, nothing to run alongside it.`,
			Site: "geocoding-api.open-meteo.com",
			Repo: "https://github.com/tamnd/geocodingmeteo-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// get: fetch one location by numeric ID (e.g. 2988507 for Paris).
	// This is the resolver op that makes Location mint-able as a URI.
	kit.Handle(app, kit.OpMeta{
		Name:     "get",
		Group:    "read",
		Single:   true,
		Resolver: true,
		Summary:  "Fetch a location by its Open-Meteo numeric ID",
		URIType:  "location",
		Args:     []kit.Arg{{Name: "id", Help: "numeric location ID"}},
	}, getLocation)

	// search: list of Location records matching a city/place name.
	kit.Handle(app, kit.OpMeta{
		Name:    "search",
		Group:   "read",
		List:    true,
		Summary: "Search for a city or location by name",
		Args:    []kit.Arg{{Name: "name", Help: "city or place name (or numeric ID)"}},
	}, searchLocations)
}

// newClient builds the client from the host-resolved config, so a host and the
// standalone binary pace and identify themselves the same way.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type getInput struct {
	ID     string  `kit:"arg"  help:"numeric location ID"`
	Client *Client `kit:"inject"`
}

type searchInput struct {
	Name     string  `kit:"arg"  help:"city or place name (or numeric ID)"`
	Count    int     `kit:"flag" help:"maximum number of results" default:"10"`
	Language string  `kit:"flag" help:"result language (ISO 639-1 code)" default:"en"`
	Client   *Client `kit:"inject"`
}

// --- handlers ---

func getLocation(ctx context.Context, in getInput, emit func(*Location) error) error {
	locs, err := in.Client.Search(ctx, in.ID, 1, "en")
	if err != nil {
		return mapErr(err)
	}
	if len(locs) == 0 {
		return errs.NotFound("location %s not found", in.ID)
	}
	return emit(locs[0])
}

func searchLocations(ctx context.Context, in searchInput, emit func(*Location) error) error {
	locs, err := in.Client.Search(ctx, in.Name, in.Count, in.Language)
	if err != nil {
		return mapErr(err)
	}
	for _, loc := range locs {
		if err := emit(loc); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: URI-native string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id) pair so
// `ant resolve` and `ant url` touch no network.
//
// All-digits input is treated as a numeric location ID; everything else is a
// query string.
func (Domain) Classify(input string) (uriType, id string, err error) {
	if input == "" {
		return "", "", errs.Usage("empty geocodingmeteo reference")
	}
	if isAllDigits(input) {
		return "location", input, nil
	}
	return "query", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	base := DefaultConfig().BaseURL
	switch uriType {
	case "location":
		return base + "/v1/search?name=" + id + "&count=1", nil
	case "query":
		return base + "/v1/search?name=" + id, nil
	default:
		return "", errs.Usage("geocodingmeteo has no resource type %q", uriType)
	}
}

// --- helpers ---

// isAllDigits reports whether s consists entirely of ASCII decimal digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// mapErr converts a library error into the kit error kind that carries the
// right exit code.
func mapErr(err error) error {
	return err
}
