package bridge

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestPushRegistrationRouteIsNotExposed(t *testing.T) {
	store, err := OpenStore("file:"+filepath.Join(t.TempDir(), "bridge.db")+"?_foreign_keys=on", DefaultGroupPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewAPI(store, fakeWhatsApp{}, fakeWhatsApp{}, fakeWhatsApp{}, t.TempDir(), "https://bridge.example.test", "api-token", "setup-token")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/push/register", nil)
	request.Header.Set("Authorization", "Bearer api-token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected removed push registration route to return 404, got %d", response.Code)
	}
}
