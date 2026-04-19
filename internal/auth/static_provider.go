package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/abartrim/sobs/internal/extensionpoints"
)

var errUnauthorized = errors.New("unauthorized")

type StaticProvider struct{}

func NewStaticProvider() extensionpoints.AuthProvider {
	return &StaticProvider{}
}

func (p *StaticProvider) Authenticate(ctx context.Context, r *http.Request) (extensionpoints.Identity, error) {
	_ = ctx
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if authz == "" {
		return extensionpoints.Identity{}, errUnauthorized
	}
	return extensionpoints.Identity{Subject: authz}, nil
}

func (p *StaticProvider) Authorize(ctx context.Context, id extensionpoints.Identity, permission string) error {
	_ = ctx
	_ = permission
	if strings.TrimSpace(id.Subject) == "" {
		return errUnauthorized
	}
	return nil
}
