package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
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
	ContextTeamID      contextKey = "team_id" // workspace ID (legacy key name)
	ContextScopes      contextKey = "scopes"
	ContextTokenID     contextKey = "token_id"
	ContextPrincipalID contextKey = "principal_id"
)

// AuthInfo is the resolved credential for a request.
type AuthInfo struct {
	WorkspaceID uuid.UUID
	Scopes      []string
	TokenID     *uuid.UUID
	PrincipalID *uuid.UUID
}

type Service struct {
	store          *store.Store
	bootstrapToken string
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

func (s *Service) Authenticate(ctx context.Context, rawToken string) (*AuthInfo, error) {
	if rawToken == s.bootstrapToken && s.bootstrapToken != "" {
		workspace, err := s.store.GetWorkspaceByName(ctx, "default")
		if err != nil {
			return nil, err
		}
		return &AuthInfo{
			WorkspaceID: workspace.ID,
			Scopes:      []string{"admin"},
		}, nil
	}

	token, err := s.store.GetTokenByHash(ctx, s.HashToken(rawToken))
	if err != nil {
		return nil, launchpad.ErrUnauthorized
	}
	if token.RevokedAt != nil {
		return nil, launchpad.ErrUnauthorized
	}
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return nil, launchpad.ErrUnauthorized
	}
	tid := token.ID
	info := &AuthInfo{
		WorkspaceID: token.WorkspaceID,
		Scopes:      token.Scopes,
		TokenID:     &tid,
	}
	if token.PrincipalID != nil {
		info.PrincipalID = token.PrincipalID
	}
	return info, nil
}

// CreateToken mints a workspace API token and a service account principal for attribution.
func (s *Service) CreateToken(ctx context.Context, workspaceName, name string, scopes []string) (string, *domain.APIToken, *domain.Principal, error) {
	workspace, err := s.store.GetWorkspaceByName(ctx, workspaceName)
	if err != nil {
		return "", nil, nil, err
	}
	if strings.TrimSpace(name) == "" {
		return "", nil, nil, fmt.Errorf("%w: token name is required", launchpad.ErrBadRequest)
	}
	plaintext, err := s.GenerateToken()
	if err != nil {
		return "", nil, nil, err
	}

	role := domain.WorkspaceRoleOperator
	for _, sc := range scopes {
		if sc == "admin" {
			role = domain.WorkspaceRoleAdmin
			break
		}
	}

	principal := &domain.Principal{
		Kind:        domain.PrincipalKindServiceAccount,
		DisplayName: name,
		Status:      domain.PrincipalStatusActive,
	}
	token := &domain.APIToken{
		WorkspaceID: workspace.ID,
		Name:        name,
		TokenHash:   s.HashToken(plaintext),
		Scopes:      scopes,
	}

	err = s.store.Transact(ctx, func(tx *sql.Tx) error {
		if err := s.store.CreatePrincipal(ctx, tx, principal); err != nil {
			return err
		}
		if err := s.store.AddWorkspaceMember(ctx, tx, &domain.WorkspaceMember{
			WorkspaceID: workspace.ID,
			PrincipalID: principal.ID,
			Role:        role,
		}); err != nil {
			return err
		}
		pid := principal.ID
		token.PrincipalID = &pid
		return s.store.CreateTokenTx(ctx, tx, token)
	})
	if err != nil {
		return "", nil, nil, err
	}
	return plaintext, token, principal, nil
}

func Middleware(authSvc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r)
			if raw == "" {
				problemUnauthorized(w, "missing bearer token")
				return
			}
			info, err := authSvc.Authenticate(r.Context(), raw)
			if err != nil {
				problemUnauthorized(w, "invalid token")
				return
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextTeamID, info.WorkspaceID)
			ctx = context.WithValue(ctx, ContextScopes, info.Scopes)
			if info.TokenID != nil {
				ctx = context.WithValue(ctx, ContextTokenID, *info.TokenID)
			}
			if info.PrincipalID != nil {
				ctx = context.WithValue(ctx, ContextPrincipalID, *info.PrincipalID)
			}
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

func TokenIDFromContext(ctx context.Context) *uuid.UUID {
	v, ok := ctx.Value(ContextTokenID).(uuid.UUID)
	if !ok || v == uuid.Nil {
		return nil
	}
	id := v
	return &id
}

func PrincipalIDFromContext(ctx context.Context) *uuid.UUID {
	v, ok := ctx.Value(ContextPrincipalID).(uuid.UUID)
	if !ok || v == uuid.Nil {
		return nil
	}
	id := v
	return &id
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
