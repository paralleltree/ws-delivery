package middleware

import (
	"net/http"

	"github.com/hnakamur/ltsvlog/v3"
)

func AuthenticationMiddleware(acceptToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authToken := getQueryParamAny(r, "authToken", "auth")
			if authToken != acceptToken {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"message":"unauthorized"}`))
				xid := GetXID(r.Context())
				ltsvlog.Logger.Info().String("xid", xid).String("event", "requestUnauthorized").Log()
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func getQueryParamAny(r *http.Request, keys ...string) string {
	for _, k := range keys {
		if v := r.URL.Query().Get(k); v != "" {
			return v
		}
	}
	return ""
}
