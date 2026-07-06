package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type contextKey string

const (
	ContextTeamID  contextKey = "team_id"
	ContextScopes  contextKey = "scopes"
	ContextTokenID contextKey = "token_id"
)

type Service struct {
	store           *store.Store
	bootstrapToken  string
}

func NewService(s *store.Store, bootstrapToken string) *Service {
	return &Service{store: s, bootstrapToken: bootstrapToken}
}

func (s *Service) HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (s *Service) GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "lp_" + hex.EncodeToString(b), nil
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (uuid.UUID, []string, error) {
	if rawToken == s.bootstrapToken && s.bootstrapToken != "" {
		workspace, err := s.store.GetWorkspaceByName(ctx, "default")
		if err != nil {
			return uuid.Nil, nil, err
		}
		return workspace.ID, []string{"admin"}, nil
	}

	token, err := s.store.GetTokenByHash(ctx, s.HashToken(rawToken))
	if err != nil {
		return uuid.Nil, nil, launchpad.ErrUnauthorized
	}
	if token.RevokedAt != nil {
		return uuid.Nil, nil, launchpad.ErrUnauthorized
	}
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return uuid.Nil, nil, launchpad.ErrUnauthorized
	}
	return token.WorkspaceID, token.Scopes, nil
}

func (s *Service) CreateToken(ctx context.Context, workspaceName, name string, scopes []string) (string, *domain.APIToken, error) {
	workspace, err := s.store.GetWorkspaceByName(ctx, workspaceName)
	if err != nil {
		return "", nil, err
	}
	plaintext, err := s.GenerateToken()
	if err != nil {
		return "", nil, err
	}
	token := &domain.APIToken{
		ID:          uuid.New(),
		WorkspaceID: workspace.ID,
		Name:        name,
		TokenHash:   s.HashToken(plaintext),
		Scopes:      scopes,
	}
	if err := s.store.CreateToken(ctx, token); err != nil {
		return "", nil, err
	}
	return plaintext, token, nil
}

func Middleware(authSvc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r)
			if raw == "" {
				problemUnauthorized(w, "missing bearer token")
				return
			}
			teamID, scopes, err := authSvc.Authenticate(r.Context(), raw)
			if err != nil {
				problemUnauthorized(w, "invalid token")
				return
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextTeamID, teamID)
			ctx = context.WithValue(ctx, ContextScopes, scopes)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes := ScopesFromContext(r.Context())
			for _, s := range scopes {
				if s == scope || s == "admin" {
					next.ServeHTTP(w, r)
					return
				}
			}
			problemForbidden(w, "missing scope: "+scope)
		})
	}
}

func TeamIDFromContext(ctx context.Context) uuid.UUID {
	v, _ := ctx.Value(ContextTeamID).(uuid.UUID)
	return v
}

func ScopesFromContext(ctx context.Context) []string {
	v, _ := ctx.Value(ContextScopes).([]string)
	return v
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

func problemUnauthorized(w http.ResponseWriter, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = fmt.Fprintf(w, `{"type":"https://launchpad.dev/errors/unauthorized","title":"Unauthorized","status":401,"detail":%q}`, detail)
}

func problemForbidden(w http.ResponseWriter, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = fmt.Fprintf(w, `{"type":"https://launchpad.dev/errors/forbidden","title":"Forbidden","status":403,"detail":%q}`, detail)
}