package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aklkbqx/wol/internal/store"
)

func TestCRUDAndExport(t *testing.T) {
	directory := t.TempDir()
	dataStore, err := store.Open(filepath.Join(directory, "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dataStore.Close()
	handler := NewServer(dataStore, "", "http://localhost:5173").Handler()

	sitePayload := `{"name":"home","broadcastAddress":"192.168.1.255","defaultPort":9}`
	siteResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/sites", sitePayload)
	if !siteResponse.Success {
		t.Fatalf("create site failed: %s", siteResponse.Message)
	}
	var site store.Site
	decodeData(t, siteResponse, &site)

	devicePayload := `{"name":"nas","macAddress":"AA:BB:CC:DD:EE:FF","ipAddress":"192.168.1.20","siteId":"` + site.ID + `","verifyPort":445,"enabled":true}`
	deviceResponse := requestJSON(t, handler, http.MethodPost, "/api/v1/devices", devicePayload)
	if !deviceResponse.Success {
		t.Fatalf("create device failed: %s", deviceResponse.Message)
	}

	bootstrapResponse := requestJSON(t, handler, http.MethodGet, "/api/v1/bootstrap", "")
	if !bootstrapResponse.Success {
		t.Fatalf("bootstrap failed: %s", bootstrapResponse.Message)
	}
	var bootstrap struct {
		Devices []store.Device `json:"devices"`
	}
	decodeData(t, bootstrapResponse, &bootstrap)
	if len(bootstrap.Devices) != 1 || bootstrap.Devices[0].MACAddress != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("unexpected bootstrap devices: %+v", bootstrap.Devices)
	}

	exportResponse := requestJSON(t, handler, http.MethodGet, "/api/v1/export", "")
	if exportResponse.Success {
		t.Fatal("export should use a raw download response, not an API envelope")
	}
	if exportResponse.Message != "raw response" {
		t.Fatalf("unexpected export helper result: %+v", exportResponse)
	}

	if _, err := os.Stat(filepath.Join(directory, "wol.db")); err != nil {
		t.Fatalf("database was not created: %v", err)
	}
}

type responseEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

func requestJSON(t *testing.T, handler http.Handler, method, path, body string) responseEnvelope {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	if body != "" {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if path == "/api/v1/export" {
		if response.Code != http.StatusOK {
			t.Fatalf("%s %s returned %d", method, path, response.Code)
		}
		return responseEnvelope{Message: "raw response"}
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %s %s response: %v; body=%s", method, path, err, response.Body.String())
	}
	return envelope
}

func decodeData(t *testing.T, response responseEnvelope, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Data, destination); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
}
