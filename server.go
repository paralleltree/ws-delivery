package wsdelivery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/hnakamur/ltsvlog/v3"
	"github.com/paralleltree/ws-delivery/lib"
	"github.com/rs/xid"
	"golang.org/x/net/websocket"
)

func wsConnectionHandler(ctx context.Context, inboxCh <-chan string) http.Handler {
	newChBuilder := newBroadcasterBuilder(inboxCh)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xid := getXID(r.Context())
		ltsvlog.Logger.Info().String("xid", xid).String("event", "wsConnected").Log()
		ch, cleanup := newChBuilder()
		defer cleanup()
		websocket.Handler(deliveryHandler(ctx, ch, cleanup)).ServeHTTP(w, r)
		ltsvlog.Logger.Info().String("xid", xid).String("event", "wsDisconnected").Log()
	})
}

func deliveryHandler(ctx context.Context, inboxCh <-chan string, cleanup func()) func(*websocket.Conn) {
	return func(c *websocket.Conn) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			<-ctx.Done()
			c.Close()
		}()

		for v := range inboxCh {
			if err := websocket.Message.Send(c, v); err != nil {
				fmt.Fprintf(os.Stderr, "send message: %v\n", err)
				cleanup()
			}
		}
	}
}

// Returns creating new channel func and cleanup func to prevent blocking channel
// 接続がない状態でも送信が止まらないように適宜スルーさせる
func newBroadcasterBuilder(inbox <-chan string) func() (<-chan string, func()) {
	l := &sync.Mutex{}
	m := map[chan<- string]struct{}{}

	go func() {
		for v := range inbox {
			// copy current living channel
			l.Lock()
			living := make([]chan<- string, 0, len(m))
			for c := range m {
				living = append(living, c)
			}
			l.Unlock()

			// send message to copied channels
			for _, c := range living {
				c <- v
			}
		}
	}()

	// returns register new channel func and cleanup func
	return func() (<-chan string, func()) {
		ch := make(chan string)
		l.Lock()
		defer l.Unlock()
		m[ch] = struct{}{}

		cleanup := func() {
			l.Lock()
			defer l.Unlock()
			if _, ok := m[ch]; !ok {
				// prevent double-closing
				return
			}
			delete(m, ch)
			close(ch)
		}

		return ch, cleanup
	}
}

type ServerConfig struct {
	Port        string
	AcceptToken string
}

func Serve(ctx context.Context, conf ServerConfig, inboxCh <-chan string) error {
	requestLogMiddleware := requestLogMiddleware()
	authMiddleware := authenticationMiddleware(conf.AcceptToken)
	baseHandlerBuilder := newHandlerBuilder(requestLogMiddleware)

	mux := http.NewServeMux()
	mux.Handle("/ws", baseHandlerBuilder(authMiddleware(wsConnectionHandler(ctx, inboxCh))))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", conf.Port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "shutdown: %v\n", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("listen and serve: %w", err)
		}
	}
	return nil
}

type contextKey string

const xidKey contextKey = "xid"

func setXID(ctx context.Context, xid string) context.Context {
	return context.WithValue(ctx, xidKey, xid)
}

func getXID(ctx context.Context) string {
	if xid, ok := ctx.Value(xidKey).(string); ok {
		return xid
	}
	return "-"
}

func requestLogMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// set transaction id
			xid := xid.New().String()
			r = r.WithContext(setXID(r.Context(), xid))

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

func authenticationMiddleware(acceptToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authToken := r.URL.Query().Get("authToken")
			if authToken != acceptToken {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"message":"unauthorized"}`))
				xid := getXID(r.Context())
				ltsvlog.Logger.Info().String("xid", xid).String("event", "requestUnauthorized").Log()
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func newHandlerBuilder(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(finalHandler http.Handler) http.Handler {
		h := finalHandler
		for i := 0; i < len(middlewares); i++ {
			h = middlewares[len(middlewares)-i-1](h)
		}
		return h
	}
}
