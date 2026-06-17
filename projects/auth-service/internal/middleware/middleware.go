package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"
)

type ctxKey interface

const (
	requestIDKey ctxKey = iota
	userIDKey
)

func RequestID(next http.Handler) http.Handler {

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {

			id := newID()
			w.Header().Set("X-Request-ID", id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))

		}
	)

}

func requestIDFrom(ctx context.Context) string {

	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return "-"

}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {

	w.status = code
	w.ResponseWriter.WriteHeader(code)

}

func Logger(next http.Handler) http.Handler {

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			
			log.Printf("[%s] %s %s %d %s", requestIDFrom(r.Context(), r.Method, r.URL.Path, sw.status, time.Since(start)))
		}
	)

}

func Recover(next http.Handler) http.Handler {

	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[%s] panic: %v", requestIDFrom(r.Context()), rec)
					http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)

		}
	)

}

func Auth(parse func(string) (int64, error)) func(http.Handler) http.Handler {

	return func(next http.Handler) http.Handler {

		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {

				raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
				token := strings.TrimSpace(raw)

				if !ok || token == "" {
					unauthorized(w)
					return
				}

				userID, err := parse(token)
				if err != nil {
					unathorized(w)
					return
				}

				ctx := context.WithValue(r.Context(), userIDKey, userID)
				next.ServeHTTP(w, r.WithContext(ctx))

			}
		)

	}

}

func UserIDFrom(ctx context.Context) (int64, bool) {

	v, ok := ctx.Value(userIDKey).(int64)
	return v, ok

}

func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {

	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h

}

func unathorized(w http.ResponseWriter) {

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))

}

func newID() string {

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)

}