package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/api/problem"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/service"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type Server struct {
	projects   *service.ProjectService
	config     *service.ConfigService
	releases   *service.ReleaseService
	changesets *service.ChangesetService
	runtime    *service.RuntimeService
	tokens     *auth.Service
	jobs       *store.Store
}

func NewServer(
	projects *service.ProjectService,
	config *service.ConfigService,
	releases *service.ReleaseService,
	changesets *service.ChangesetService,
	runtime *service.RuntimeService,
	tokens *auth.Service,
	jobs *store.Store,
) *Server {
	return &Server{
		projects:   projects,
		config:     config,
		releases:   releases,
		changesets: changesets,
		runtime:    runtime,
		tokens:     tokens,
		jobs:       jobs,
	}
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
		r.With(auth.RequireScope("project:read")).Get("/jobs/{id}", s.getJob)

		r.With(auth.RequireScope("project:write")).Post("/projects", s.createProject)
		r.With(auth.RequireScope("project:read")).Get("/projects", s.listProjects)
		r.With(auth.RequireScope("project:read")).Get("/projects/{project}", s.getProject)

		r.With(auth.RequireScope("project:read")).Get("/projects/{project}/config", s.getConfig)
		r.With(auth.RequireScope("project:write")).Patch("/projects/{project}/config", s.patchConfig)

		r.With(auth.RequireScope("project:read")).Get("/projects/{project}/processes", s.listProcesses)
		r.With(auth.RequireScope("project:read")).Get("/projects/{project}/logs", s.getLogs)

		r.With(auth.RequireScope("project:read")).Get("/projects/{project}/environments", s.listEnvironments)
		r.With(auth.RequireScope("project:write")).Post("/projects/{project}/environments", s.createEnvironment)
		r.With(auth.RequireScope("project:read")).Get("/projects/{project}/environments/{name}", s.getEnvironment)

		r.With(auth.RequireScope("deploy")).Post("/projects/{project}/releases", s.createRelease)
		r.With(auth.RequireScope("project:read")).Get("/projects/{project}/releases", s.listReleases)
		r.With(auth.RequireScope("deploy")).Post("/projects/{project}/rollback", s.rollback)

		r.With(auth.RequireScope("project:read")).Get("/projects/{project}/changeset", s.getChangeset)
		r.With(auth.RequireScope("project:write")).Post("/projects/{project}/changeset/changes", s.stageChanges)
		r.With(auth.RequireScope("project:write")).Delete("/projects/{project}/changeset", s.discardChangeset)
		r.With(auth.RequireScope("deploy")).Post("/projects/{project}/changeset/push", s.pushChangeset)
	})
	return r
}

func environmentFromRequest(r *http.Request) string {
	if v := r.Header.Get("X-Launchpad-Environment"); v != "" {
		return v
	}
	return service.DefaultEnvironment
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var input service.CreateProjectInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	project, err := s.projects.CreateProject(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, projectResponse(project))
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.projects.ListProjects(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]projectDTO, 0, len(projects))
	for i := range projects {
		out = append(out, projectResponse(&projects[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	project, err := s.projects.GetProject(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, projectResponse(project))
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	layer := r.URL.Query().Get("layer")
	vars, err := s.config.GetConfig(r.Context(), chi.URLParam(r, "project"), environmentFromRequest(r), layer)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (s *Server) patchConfig(w http.ResponseWriter, r *http.Request) {
	var updates map[string]*string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	vars, err := s.config.PatchConfig(r.Context(), chi.URLParam(r, "project"), environmentFromRequest(r), updates)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (s *Server) listEnvironments(w http.ResponseWriter, r *http.Request) {
	envs, err := s.projects.ListEnvironments(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]environmentDTO, 0, len(envs))
	for i := range envs {
		out = append(out, environmentResponse(&envs[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createEnvironment(w http.ResponseWriter, r *http.Request) {
	var input service.CreateEnvironmentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	env, err := s.projects.CreateEnvironment(r.Context(), chi.URLParam(r, "project"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, environmentResponse(env))
}

func (s *Server) getEnvironment(w http.ResponseWriter, r *http.Request) {
	env, err := s.projects.GetEnvironment(r.Context(), chi.URLParam(r, "project"), chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, environmentResponse(env))
}

func (s *Server) listProcesses(w http.ResponseWriter, r *http.Request) {
	processes, err := s.projects.ListProcesses(r.Context(), chi.URLParam(r, "project"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]processDTO, 0, len(processes))
	for _, p := range processes {
		out = append(out, processResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		problem.NotImplemented(w, "logs not configured")
		return
	}
	process := r.URL.Query().Get("process")
	if process == "" {
		process = "web"
	}
	rc, err := s.runtime.Logs(r.Context(), chi.URLParam(r, "project"), environmentFromRequest(r), process)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (s *Server) createRelease(w http.ResponseWriter, r *http.Request) {
	var input service.CreateReleaseInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	result, err := s.releases.CreateRelease(r.Context(), chi.URLParam(r, "project"), environmentFromRequest(r), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, releaseJobResponse(result))
}

func (s *Server) rollback(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Version     int    `json:"version"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	result, err := s.releases.Rollback(r.Context(), chi.URLParam(r, "project"), environmentFromRequest(r), input.Version, input.Description)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, releaseJobResponse(result))
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	releases, err := s.releases.ListReleases(r.Context(), chi.URLParam(r, "project"), environmentFromRequest(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]releaseDTO, 0, len(releases))
	for _, rel := range releases {
		out = append(out, releaseWithDeploymentsResponse(rel))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getChangeset(w http.ResponseWriter, r *http.Request) {
	cs, err := s.changesets.GetChangeset(r.Context(), chi.URLParam(r, "project"), environmentFromRequest(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, changesetViewResponse(cs))
}

func (s *Server) stageChanges(w http.ResponseWriter, r *http.Request) {
	var input service.StageChangesInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	cs, err := s.changesets.StageChanges(r.Context(), chi.URLParam(r, "project"), environmentFromRequest(r), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, changesetViewResponse(cs))
}

func (s *Server) discardChangeset(w http.ResponseWriter, r *http.Request) {
	if err := s.changesets.DiscardChangeset(r.Context(), chi.URLParam(r, "project")); err != nil {
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
	result, err := s.changesets.PushChangeset(r.Context(), chi.URLParam(r, "project"), environmentFromRequest(r), input)
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
	writeJSON(w, http.StatusOK, jobResponse(job))
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name      string   `json:"name"`
		Workspace string   `json:"workspace"`
		Team      string   `json:"team"`
		Scopes    []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		problem.BadRequest(w, "invalid json")
		return
	}
	workspace := input.Workspace
	if workspace == "" {
		workspace = input.Team
	}
	if workspace == "" {
		workspace = "default"
	}
	plaintext, token, err := s.tokens.CreateToken(r.Context(), workspace, input.Name, input.Scopes)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": token.ID, "name": token.Name, "workspace": workspace,
		"scopes": token.Scopes, "token": plaintext,
	})
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
		problem.Internal(w, "internal server error")
	}
}
