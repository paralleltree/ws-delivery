package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

func wsConnectionHandler(ctx context.Context, inboxCh <-chan string) http.Handler {
	newChBuilder := newBroadcasterBuilder(inboxCh)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ch, cleanup := newChBuilder()
		defer cleanup()
		websocket.Handler(deliveryHandler(ctx, ch, cleanup)).ServeHTTP(w, r)
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

func serve(ctx context.Context, conf ServerConfig, inboxCh <-chan string) error {
	authMiddleware := authenticationMiddleware(conf.AcceptToken)
	baseHandlerBuilder := newHandlerBuilder(authMiddleware)

	mux := http.NewServeMux()
	mux.Handle("/ws", baseHandlerBuilder(wsConnectionHandler(ctx, inboxCh)))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", conf.Port),
		Handler: mux,
	}

	serverShutdown := make(chan error)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				serverShutdown <- fmt.Errorf("listen and serve: %w", err)
			}
		}
		close(serverShutdown)
	}()

	select {
	case <-ctx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutdown server")
		}

	case err := <-serverShutdown:
		if err != nil {
			return fmt.Errorf("shutting down server: %w", err)
		}
	}

	return nil
}

func authenticationMiddleware(acceptToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authToken := r.URL.Query().Get("authToken")
			if authToken != acceptToken {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"message":"unauthorized"}`))
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
