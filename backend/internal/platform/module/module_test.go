package module_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/module"
)

type fake struct{ name string }

func (f fake) Name() string { return f.name }
func (f fake) Routes(r *module.Router) {
	r.Public("GET /v1/fake/ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	r.Platform("GET /v1/fake/items", "platform.fake.read", func(http.ResponseWriter, *http.Request) {})
	r.Tenant("GET /v1/tenant/fake", "tenant.fake.read", func(http.ResponseWriter, *http.Request) {})
}

func TestMountRecordsAndServes(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	r := module.NewRouter(module.Deps{Mux: mux})
	if err := r.Mount(fake{name: "fake"}); err != nil {
		t.Fatal(err)
	}
	routes := r.Routes()
	// Tenant route skipped: no resolver (tenancy off).
	if len(routes) != 2 {
		t.Fatalf("routes = %+v", routes)
	}
	if routes[0].Module != "fake" || routes[1].Access != module.AccessPublic {
		t.Fatalf("routes = %+v", routes)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/fake/ping", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("ping = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/fake/items", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("platform route without token = %d, want 401", rec.Code)
	}
}

func TestMountRejectsDuplicateNames(t *testing.T) {
	t.Parallel()
	r := module.NewRouter(module.Deps{})
	if err := r.Mount(fake{name: "a"}, fake{name: "a"}); err == nil {
		t.Fatal("duplicate module name accepted")
	}
}
