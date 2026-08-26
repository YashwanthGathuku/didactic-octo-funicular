package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"sentinel-gateway/internal/auth"
	"sentinel-gateway/internal/lens"
)

type lensCreateInvestigationRequest struct {
	Title string `json:"title"`
}

type lensAddNodeRequest struct {
	ParentNodeID string           `json:"parent_node_id,omitempty"`
	Question     string           `json:"question"`
	QueryIntent  lens.QueryIntent `json:"query_intent"`
}

func registerLensRoutes(r chi.Router, db *sql.DB) {
	svc := lens.NewService(db)

	r.Route("/lens", func(r chi.Router) {
		r.Get("/datasets", func(w http.ResponseWriter, req *http.Request) {
			if _, serr := resolveScope(req, auth.PermReadTenant); serr != nil {
				serr.write(w)
				return
			}
			writeJSON(w, http.StatusOK, svc.ListDatasets())
		})

		r.Post("/query", func(w http.ResponseWriter, req *http.Request) {
			scope, serr := resolveScope(req, auth.PermReadTenant)
			if serr != nil {
				serr.write(w)
				return
			}
			var intent lens.QueryIntent
			dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 64*1024))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&intent); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_query_intent", "detail": err.Error()})
				return
			}
			result, err := svc.Execute(req.Context(), scope.TenantID(), intent)
			if err != nil {
				writeLensError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, result)
		})

		r.Post("/investigations", func(w http.ResponseWriter, req *http.Request) {
			scope, serr := resolveScope(req, auth.PermManageAnalytics)
			if serr != nil {
				serr.write(w)
				return
			}
			var body lensCreateInvestigationRequest
			dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 16*1024))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_investigation", "detail": err.Error()})
				return
			}
			inv, err := svc.CreateInvestigation(req.Context(), scope.TenantID(), body.Title, scope.ActorID())
			if err != nil {
				writeLensError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, inv)
		})

		r.Get("/investigations/{id}", func(w http.ResponseWriter, req *http.Request) {
			scope, serr := resolveScope(req, auth.PermReadTenant)
			if serr != nil {
				serr.write(w)
				return
			}
			inv, err := svc.GetInvestigation(req.Context(), scope.TenantID(), chi.URLParam(req, "id"))
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
				return
			}
			if err != nil {
				writeLensError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, inv)
		})

		r.Post("/investigations/{id}/nodes", func(w http.ResponseWriter, req *http.Request) {
			scope, serr := resolveScope(req, auth.PermManageAnalytics)
			if serr != nil {
				serr.write(w)
				return
			}
			var body lensAddNodeRequest
			dec := json.NewDecoder(http.MaxBytesReader(w, req.Body, 96*1024))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_investigation_node", "detail": err.Error()})
				return
			}
			node, err := svc.AddNode(req.Context(), scope.TenantID(), chi.URLParam(req, "id"), body.ParentNodeID, body.Question, scope.ActorID(), body.QueryIntent)
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
				return
			}
			if err != nil {
				writeLensError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, node)
		})
	})
}

func writeLensError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, lens.ErrUnknownDataset),
		errors.Is(err, lens.ErrUnknownField),
		errors.Is(err, lens.ErrUnsafeQuery),
		errors.Is(err, lens.ErrInvalidTimeRange):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "lens_query_rejected", "detail": err.Error()})
	case errors.Is(err, sql.ErrNoRows):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
	default:
		// Do not expose SQL/schema/provider error detail to callers.
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "lens_unavailable"})
	}
}
