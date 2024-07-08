package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/net/websocket"
)

func wsConnectionHandler(ctx context.Context, inboxCh <-chan string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		websocket.Handler(deliveryHandler(ctx, inboxCh)).ServeHTTP(w, r)
	})
}

func deliveryHandler(ctx context.Context, inboxCh <-chan string) func(*websocket.Conn) {
	return func(c *websocket.Conn) {
		defer c.Close()

		for v := range inboxCh {
			if err := websocket.Message.Send(c, v); err != nil {
				fmt.Fprintf(os.Stderr, "send message: %v", err)
			}
		}
	}
}

func serve(ctx context.Context, inboxCh <-chan string) error {
	acceptToken := os.Getenv("ACCEPT_TOKEN")
	authMiddleware := authenticateMiddleware(acceptToken)
	baseHandlerBuilder := newHandlerBuilder(authMiddleware)

	mux := http.NewServeMux()
	mux.Handle("/ws", baseHandlerBuilder(wsConnectionHandler(ctx, inboxCh)))

	port := 8080
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
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

	<-ctx.Done()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server")
	}
	if err := <-serverShutdown; err != nil {
		return fmt.Errorf("shutting down server: %w", err)
	}

	return nil
}

func authenticateMiddleware(acceptToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader != acceptToken {
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
		for i := 0; i < len(middlewares)-1; i++ {
			h = middlewares[len(middlewares)-i-1](h)
		}
		return h
	}
}
