package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rubenwoldhuis/recipes/internal/config"
	"github.com/rubenwoldhuis/recipes/internal/database"
	"github.com/rubenwoldhuis/recipes/internal/handlers"
	"github.com/rubenwoldhuis/recipes/internal/llm"
	"github.com/rubenwoldhuis/recipes/internal/server"
	"github.com/rubenwoldhuis/recipes/internal/tools"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.Database.ConnString())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := database.RunMigrations(ctx, pool); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	ollamaClient := llm.NewClient(cfg.Ollama.Host, cfg.Ollama.Model, cfg.Ollama.GenerationTimeout)
	if err := ollamaClient.EnsureModel(ctx); err != nil {
		log.Printf("Warning: could not ensure model: %v", err)
	}

	queries := database.NewQueries(pool)

	webSearcher := tools.NewWebSearcher(cfg.Search.Timeout)
	dbSearcher := tools.NewDBSearcher(queries)

	var edamamClient *tools.EdamamClient
	if cfg.Edamam.Enabled() {
		edamamClient = tools.NewEdamamClient(cfg.Edamam.AppID, cfg.Edamam.AppKey, cfg.Search.Timeout)
	}

	executor := tools.NewExecutor(webSearcher, dbSearcher, edamamClient)
	orchestrator := llm.NewOrchestrator(ollamaClient, executor, cfg.Ollama.MaxToolIterations, cfg.Edamam.Enabled())

	recipeHandler := handlers.NewRecipeHandler(queries)
	generateHandler := handlers.NewGenerateHandler(orchestrator, queries)

	router := server.NewRouter(recipeHandler, generateHandler, cfg.Server.CORSOrigin)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("Shutting down...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	log.Printf("Server starting on :%d (model: %s)", cfg.Server.Port, cfg.Ollama.Model)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
