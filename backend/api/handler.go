package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/imbroyury/border/backend/db"
)

// Querier abstracts the DB queries for testability.
type Querier interface {
	GetZones(ctx context.Context) ([]db.ZoneWithCount, error)
	GetSnapshots(ctx context.Context, zoneID string, from, to time.Time) ([]db.SnapshotPoint, error)
	GetCurrentVehicles(ctx context.Context, zoneID string) ([]db.VehicleRow, error)
	GetVehicleHistory(ctx context.Context, zoneID string, from, to time.Time) ([]db.VehicleRow, error)
}

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	db     Querier
	logger *slog.Logger
}

// NewHandler creates a new Handler.
func NewHandler(db Querier, logger *slog.Logger) *Handler {
	return &Handler{db: db, logger: logger}
}

// NewRouter creates a chi router with all routes and middleware.
func NewRouter(h *Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		ExposedHeaders:   []string{},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Route("/api", func(r chi.Router) {
		r.Get("/zones", h.GetZones)
		r.Get("/zones/{id}/snapshots", h.GetSnapshots)
		r.Get("/zones/{id}/vehicles", h.GetCurrentVehicles)
		r.Get("/zones/{id}/vehicles/history", h.GetVehicleHistory)
	})

	return r
}

func (h *Handler) GetZones(w http.ResponseWriter, r *http.Request) {
	zones, err := h.db.GetZones(r.Context())
	if err != nil {
		h.logger.Error("get zones", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, zones)
}

func (h *Handler) GetSnapshots(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "id")

	fromStr := r.URL.Query().Get("from")
	if fromStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'from' parameter"})
		return
	}
	toStr := r.URL.Query().Get("to")
	if toStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'to' parameter"})
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'from' date format, expected RFC3339"})
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'to' date format, expected RFC3339"})
		return
	}

	points, err := h.db.GetSnapshots(r.Context(), zoneID, from, to)
	if err != nil {
		h.logger.Error("get snapshots", "error", err, "zone_id", zoneID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, points)
}

func (h *Handler) GetCurrentVehicles(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "id")

	vehicles, err := h.db.GetCurrentVehicles(r.Context(), zoneID)
	if err != nil {
		h.logger.Error("get current vehicles", "error", err, "zone_id", zoneID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, vehicles)
}

func (h *Handler) GetVehicleHistory(w http.ResponseWriter, r *http.Request) {
	zoneID := chi.URLParam(r, "id")

	fromStr := r.URL.Query().Get("from")
	if fromStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'from' parameter"})
		return
	}
	toStr := r.URL.Query().Get("to")
	if toStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'to' parameter"})
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'from' date format, expected RFC3339"})
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid 'to' date format, expected RFC3339"})
		return
	}

	vehicles, err := h.db.GetVehicleHistory(r.Context(), zoneID, from, to)
	if err != nil {
		h.logger.Error("get vehicle history", "error", err, "zone_id", zoneID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, vehicles)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("write json response", "error", err)
	}
}
