package router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hashchain/internal/config"
	"hashchain/internal/domain"
	"hashchain/internal/handler"
	"hashchain/internal/repository/memory"
	"hashchain/internal/router"
	"hashchain/internal/service"
)

func newServer(t *testing.T) http.Handler {
	t.Helper()
	cfg := config.Config{
		Params:       domain.Params{InitialDifficulty: 10, MinDifficulty: 8, MaxDifficulty: 14, TargetBlockTime: time.Second},
		MineTimeout:  5 * time.Second,
		MaxDataBytes: 32,
	}
	svc, err := service.NewChainService(context.Background(), memory.NewChainStore(), cfg.Params)
	if err != nil {
		t.Fatal(err)
	}
	return router.New(handler.NewChainHandler(svc, cfg))
}

func do(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestMineTamperValidateOverHTTP(t *testing.T) {
	srv := newServer(t)

	for _, data := range []string{"one", "two", "three"} {
		if code, body := do(t, srv, "POST", "/blocks", `{"data":"`+data+`"}`); code != http.StatusCreated {
			t.Fatalf("mine %q: status %d %v", data, code, body)
		}
	}

	code, latest := do(t, srv, "GET", "/blocks/latest", "")
	if code != http.StatusOK || latest["height"].(float64) != 3 || latest["data"] != "three" {
		t.Fatalf("latest: %d %v", code, latest)
	}

	if code, v := do(t, srv, "GET", "/chain/validate", ""); code != http.StatusOK || v["valid"] != true {
		t.Fatalf("validate before tamper: %d %v", code, v)
	}

	code, res := do(t, srv, "POST", "/debug/tamper/1", `{"data":"evil","mode":"remine"}`)
	if code != http.StatusOK {
		t.Fatalf("tamper: %d %v", code, res)
	}
	after := res["validation_after"].(map[string]any)
	if after["valid"] != false || after["first_invalid_height"].(float64) != 2 || after["rule"] != domain.RulePrevHash {
		t.Fatalf("validation after remine tamper: %v", after)
	}
}

func TestHTTPErrors(t *testing.T) {
	srv := newServer(t)
	tests := []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/blocks/99", "", http.StatusNotFound},
		{"GET", "/blocks/abc", "", http.StatusBadRequest},
		{"GET", "/blocks?limit=0", "", http.StatusBadRequest},
		{"POST", "/blocks", `{"data":`, http.StatusBadRequest},
		{"POST", "/blocks", `{"data":"this string is longer than thirty-two bytes"}`, http.StatusBadRequest},
		{"POST", "/blocks", `{"unknown":1}`, http.StatusBadRequest},
		{"POST", "/debug/tamper/0", `{"mode":"sneaky"}`, http.StatusBadRequest},
		{"POST", "/debug/tamper/42", `{"mode":"body"}`, http.StatusNotFound},
	}
	for _, tt := range tests {
		if code, body := do(t, srv, tt.method, tt.path, tt.body); code != tt.want {
			t.Errorf("%s %s %s: status %d, want %d (%v)", tt.method, tt.path, tt.body, code, tt.want, body)
		}
	}
}

func TestMinerStartConflictAndStop(t *testing.T) {
	srv := newServer(t)
	if code, _ := do(t, srv, "POST", "/miner/start", ""); code != http.StatusAccepted {
		t.Fatalf("start: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/miner/start", `{"max_blocks":1}`); code != http.StatusConflict {
		t.Fatalf("second start: %d", code)
	}
	if code, body := do(t, srv, "POST", "/miner/stop", ""); code != http.StatusOK || body["stopped"] != true {
		t.Fatalf("stop: %d %v", code, body)
	}
}
