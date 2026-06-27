package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/api/problem"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/service"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type Server struct {
	apps       *service.AppService
	releases   *service.ReleaseService
	scale      *service.ScaleService
	changesets *service.ChangesetService
	tokens     *auth.Service
	jobs       *store.Store
}

func NewServer(
	apps *service.AppService,
	releases *service.ReleaseService,
	scale *service.ScaleService,
	changesets *service.ChangesetService,
	tokens *auth.Service,
	jobs *store.Store,
) *Server {
	return &Server{apps: apps, releases: releases, scale: scale, changesets: changesets, tokens: tokens, jobs: jobs}
}

func (s *Server) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/v1", func(r chi.Router) {
		r.Use(auth.Middleware(s.tokens))

		r.With(auth.RequireScope("admin")).Post("/tokens", s.createToken)

		r.With(auth.RequireScope("app:write")).Post("/apps", s.createApp)
		r.With(auth.RequireScope("app:read")).Get("/apps", s.listApps)
		r.With(auth.RequireScope("app:read")).Get("/apps/{app}", s.getApp)

		r.With(auth.RequireScope("app:write")).Patch("/apps/{app}/config-vars", s.patchConfigVars)
		r.With(auth.RequireScope("app:read")).Get("/apps/{app}/config-vars", s.getConfigVars)

		r.With(auth.RequireScope("app:read")).Get("/apps/{app}/processes", s.listProcesses)
		r.With(auth.RequireScope("scale")).Patch("/apps/{app}/processes/{process}/scale", s.scaleProcess)

		r.With(auth.RequireScope("deploy")).Post("/apps/{app}/releases", s.createRelease)
		r.With(auth.RequireScope("app:read")).Get("/apps/{app}/releases", s.listReleases)
		r.With(auth.RequireScope("deploy")).Post("/apps/{app}/releases/{version}/rollback", s.rollbackRelease)

		r.With(auth.RequireScope("app:read")).Get("/apps/{app}/changeset", s.getChangeset)
		r.With(auth.RequireScope("app:write")).Post("/apps/{app}/changeset/changes", s.stageChanges)
		r.With(auth.RequireScope("app:write")).Delete("/apps/{app}/changeset", s.discardChangeset)
		r.With(auth.RequireScope("deploy")).Post("/apps/{app}/changeset/push", s.pushChangeset)

		r.With(auth.RequireScope("app:read")).Get("/jobs/{id}", s.getJob)
	})
	return r
}

func (s *Server) createApp(w http.ResponseWriter, r *http.Request) {
	var input service.CreateAppInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	app, err := s.apps.CreateApp(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, appResponse(app))
}

func (s *Server) listApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.apps.ListApps(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]any, 0, len(apps))
	for i := range apps {
		out = append(out, appResponse(&apps[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getApp(w http.ResponseWriter, r *http.Request) {
	app, err := s.apps.GetApp(r.Context(), chi.URLParam(r, "app"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, appResponse(app))
}

func (s *Server) getConfigVars(w http.ResponseWriter, r *http.Request) {
	vars, err := s.apps.GetConfigVars(r.Context(), chi.URLParam(r, "app"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (s *Server) patchConfigVars(w http.ResponseWriter, r *http.Request) {
	var updates map[string]*string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	vars, err := s.apps.PatchConfigVars(r.Context(), chi.URLParam(r, "app"), updates)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (s *Server) listProcesses(w http.ResponseWriter, r *http.Request) {
	processes, err := s.apps.ListProcesses(r.Context(), chi.URLParam(r, "app"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, processes)
}

func (s *Server) scaleProcess(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Quantity int `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	result, err := s.scale.ScaleProcess(r.Context(), chi.URLParam(r, "app"), chi.URLParam(r, "process"), input.Quantity)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"process": result.Process,
		"job":     result.Job,
	})
}

func (s *Server) createRelease(w http.ResponseWriter, r *http.Request) {
	var input service.CreateReleaseInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	result, err := s.releases.CreateRelease(r.Context(), chi.URLParam(r, "app"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, releaseJobResponse(result))
}

func (s *Server) rollbackRelease(w http.ResponseWriter, r *http.Request) {
	version, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil {
		problem.BadRequest(w, "invalid release version")
		return
	}
	result, err := s.releases.RollbackRelease(r.Context(), chi.URLParam(r, "app"), version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, releaseJobResponse(result))
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	releases, err := s.releases.ListReleases(r.Context(), chi.URLParam(r, "app"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, releases)
}

func (s *Server) getChangeset(w http.ResponseWriter, r *http.Request) {
	cs, err := s.changesets.GetChangeset(r.Context(), chi.URLParam(r, "app"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) stageChanges(w http.ResponseWriter, r *http.Request) {
	var input service.StageChangesInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	cs, err := s.changesets.StageChanges(r.Context(), chi.URLParam(r, "app"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) discardChangeset(w http.ResponseWriter, r *http.Request) {
	if err := s.changesets.DiscardChangeset(r.Context(), chi.URLParam(r, "app")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) pushChangeset(w http.ResponseWriter, r *http.Request) {
	var input service.PushChangesetInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	result, err := s.changesets.PushChangeset(r.Context(), chi.URLParam(r, "app"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, releaseJobResponse(result))
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		problem.BadRequest(w, "invalid job id")
		return
	}
	job, err := s.jobs.GetJob(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name   string   `json:"name"`
		Team   string   `json:"team"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	if input.Team == "" {
		input.Team = "default"
	}
	plaintext, token, err := s.tokens.CreateToken(r.Context(), input.Team, input.Name, input.Scopes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": token.ID, "name": token.Name, "team": input.Team,
		"scopes": token.Scopes, "token": plaintext,
	})
}

func releaseJobResponse(result *service.CreateReleaseResult) map[string]any {
	return map[string]any{
		"deployment": map[string]any{
			"id": result.Deployment.ID, "status": result.Deployment.Status,
			"release": map[string]any{"version": result.Release.Version},
		},
		"job": map[string]any{
			"id": result.Job.ID, "type": result.Job.Type, "status": result.Job.Status,
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, launchpad.ErrNotFound):
		problem.NotFound(w, err.Error())
	case errors.Is(err, launchpad.ErrConflict):
		problem.Conflict(w, err.Error())
	case errors.Is(err, launchpad.ErrBadRequest):
		problem.BadRequest(w, err.Error())
	case errors.Is(err, launchpad.ErrNotImplemented):
		problem.NotImplemented(w, err.Error())
	default:
		problem.Internal(w, err.Error())
	}
}

func appResponse(app *domain.App) map[string]any {
	return map[string]any{
		"id": app.ID, "name": app.Name, "status": app.Status,
		"target_type": app.TargetType, "created_at": app.CreatedAt,
	}
}