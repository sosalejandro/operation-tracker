package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	// "github.com/gorilla/mux"
	operationtracker "github.com/sosalejandro/operation-tracker/pkg"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	tracker := operationtracker.NewOperationTracker(redisClient, logger)
	webSocketHandler := operationtracker.NewWebSocketHandler(tracker, logger)
	httpHandler := operationtracker.NewHTTPHandler(tracker, logger)

	r := http.NewServeMux()
	r.HandleFunc("GET /ws", webSocketHandler.ServeWebSocket)
	r.HandleFunc("GET /operation/status", httpHandler.GetOperationStatus)
	r.HandleFunc("GET /notifications/unread", httpHandler.ListUnreadNotifications)
	r.HandleFunc("POST /operation/read", httpHandler.MarkOperationAsRead)

	srv := &http.Server{
		Handler:      r,
		Addr:         "0.0.0.0:8080",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(ctx)

	logger.Info("Shutting down")
	os.Exit(0)
}
