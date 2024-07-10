package wsdelivery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/hnakamur/ltsvlog/v3"
	"github.com/paralleltree/ws-delivery/middleware"
	"golang.org/x/net/websocket"
)

func wsConnectionHandler(inboxCh <-chan string) http.Handler {
	newChBuilder := newBroadcasterBuilder(inboxCh)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		xid := middleware.GetXID(r.Context())
		ltsvlog.Logger.Info().String("xid", xid).String("event", "wsConnected").Log()
		ch, cleanup := newChBuilder()
		defer cleanup()
		websocket.Handler(deliveryHandler(ch, cleanup)).ServeHTTP(w, r)
		ltsvlog.Logger.Info().String("xid", xid).String("event", "wsDisconnected").Log()
	})
}

func deliveryHandler(inboxCh <-chan string, cleanup func()) func(*websocket.Conn) {
	return func(c *websocket.Conn) {
		ctx, cancel := context.WithCancel(context.Background())
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
	requestLogMiddleware := middleware.RequestLogMiddleware()
	authMiddleware := middleware.AuthenticationMiddleware(conf.AcceptToken)
	baseHandlerBuilder := newHandlerBuilder(requestLogMiddleware)
	rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"not found"}`))
	})

	mux := http.NewServeMux()
	mux.Handle("/", baseHandlerBuilder(rootHandler))
	mux.Handle("/ws", baseHandlerBuilder(authMiddleware(wsConnectionHandler(inboxCh))))

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

func newHandlerBuilder(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(finalHandler http.Handler) http.Handler {
		h := finalHandler
		for i := 0; i < len(middlewares); i++ {
			h = middlewares[len(middlewares)-i-1](h)
		}
		return h
	}
}
