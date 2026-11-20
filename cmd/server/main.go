// Command server runs the mini checkout: go run ./cmd/server
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JaipreethTiruvaipati/fynd-mini-checkout-sdet/internal/checkout"
	"github.com/JaipreethTiruvaipati/fynd-mini-checkout-sdet/internal/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	delay := flag.Duration("order-delay", 600*time.Millisecond, "simulated order processing time")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	svc := checkout.NewService(*delay)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           http.TimeoutHandler(web.NewHandler(svc, log), web.RequestTimeout, `{"error":"timeout"}`),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Info("mini checkout running", "url", "http://"+*addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("server stopped")
}
