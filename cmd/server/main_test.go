package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManifestContentType(t *testing.T) {
	mux := http.NewServeMux()
	registerSPA(mux)
	request := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("manifest状态码错误：%d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/manifest+json" {
		t.Fatalf("manifest Content-Type错误：%s", contentType)
	}
}
