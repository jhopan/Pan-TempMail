package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"receiver/api"
)

func TestStaticAppIsServedAndRevalidates(t *testing.T) {
	router := api.SetupRouter()
	for _, path := range []string{"/app/", "/app/admin.html", "/app/admin-nav.js", "/app/app.js"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: dapat %d, ingin 200", path, recorder.Code)
		}
		if got := recorder.Header().Get("Cache-Control"); !strings.Contains(got, "no-cache") {
			t.Fatalf("%s: Cache-Control = %q, ingin memuat no-cache", path, got)
		}
	}
}

// Panel yang dikenali admin-nav.js harus ada di markup yang disajikan; kalau tidak,
// halaman versi lama akan menyembunyikan semua panel dan layar jadi kosong.
func TestAdminPanelMarkupCarriesEveryPanel(t *testing.T) {
	router := api.SetupRouter()
	request := httptest.NewRequest(http.MethodGet, "/app/admin.html", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	body := recorder.Body.String()

	for _, panel := range []string{"status", "server", "diagnostics", "users", "naming", "cloudflare", "domains", "tunnel", "copies", "import"} {
		if !strings.Contains(body, `data-panel="`+panel+`"`) {
			t.Errorf("panel %q tidak ada di admin.html", panel)
		}
	}
	if strings.Count(body, `<section class="panel card" data-panel=`) != 10 {
		t.Errorf("jumlah panel != 10")
	}
	if strings.Count(body, `<section class="panel card" data-panel="status"`) == 0 {
		t.Errorf("panel status wajib ada sebagai default")
	}
	if strings.Count(body, "hidden>") < 9 {
		t.Errorf("panel selain status harus hidden secara default agar tidak berkedip")
	}
}
