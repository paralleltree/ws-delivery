package middleware

import (
	"math"
	"net/http"
	"time"

	"github.com/hnakamur/ltsvlog/v3"
	"github.com/paralleltree/ws-delivery/lib"
	"github.com/rs/xid"
)

func RequestLogMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// set transaction id
			xid := xid.New().String()
			r = r.WithContext(SetXID(r.Context(), xid))

			// logging request
			remote, err := lib.ResolveClientIP(r)
			if err != nil {
				remote = "-"
			}
			ltsvlog.Logger.Info().
				String("xid", xid).
				String("event", "requestHandling").
				String("method", r.Method).
				String("path", r.URL.Path).
				String("url", r.URL.String()).
				String("remote", remote).
				String("useragent", r.UserAgent()).
				Log()

			begin := time.Now()
			next.ServeHTTP(w, r)

			// logging duration
			duration := float64(time.Since(begin) / time.Millisecond)
			ltsvlog.Logger.Info().
				String("xid", xid).
				String("event", "requestHandled").
				Float64("duration", math.Round(duration)).
				Log()
		})
	}
}
