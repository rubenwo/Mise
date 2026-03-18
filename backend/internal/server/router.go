package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/rubenwoldhuis/recipes/internal/frontend"
	"github.com/rubenwoldhuis/recipes/internal/handlers"
)

func NewRouter(h *handlers.RecipeHandler, g *handlers.GenerateHandler, corsOrigin string) *chi.Mux {
	r := chi.NewRouter()

	r.Use(LoggingMiddleware)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{corsOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Route("/api", func(r chi.Router) {
		r.Get("/recipes", h.List)
		r.Post("/recipes", h.Create)
		r.Post("/recipes/search", h.Search)
		r.Get("/recipes/{id}", h.Get)
		r.Delete("/recipes/{id}", h.Delete)

		r.Post("/generate/single", g.Single)
		r.Post("/generate/batch", g.Batch)
		r.Post("/generate/refine", g.Refine)
	})

	r.Handle("/*", frontend.Handler())

	return r
}
