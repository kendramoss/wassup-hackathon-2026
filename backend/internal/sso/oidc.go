package sso

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// providerTTL is how long a discovered OIDC provider is reused.
const providerTTL = 15 * time.Minute

type providerCache struct {
	mu sync.Mutex
	m  map[string]cachedProvider
}

type cachedProvider struct {
	p       *oidc.Provider
	fetched time.Time
}

func (s *Service) provider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	s.providers.mu.Lock()
	if c, ok := s.providers.m[issuer]; ok && s.now().Sub(c.fetched) < providerTTL {
		s.providers.mu.Unlock()
		return c.p, nil
	}
	s.providers.mu.Unlock()
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, s.http), issuer)
	if err != nil {
		return nil, fmt.Errorf("discover issuer: %w", err)
	}
	s.providers.mu.Lock()
	s.providers.m[issuer] = cachedProvider{p: p, fetched: s.now()}
	s.providers.mu.Unlock()
	return p, nil
}

func (s *Service) oauthConfig(ctx context.Context, c Connection) (*oauth2.Config, *oidc.Provider, error) {
	p, err := s.provider(ctx, c.OIDCIssuer)
	if err != nil {
		return nil, nil, err
	}
	secret := ""
	if c.OIDCSecret != "" {
		if secret, err = s.keys.Open(c.OIDCSecret); err != nil {
			return nil, nil, err
		}
	}
	return &oauth2.Config{
		ClientID:     c.OIDCClientID,
		ClientSecret: secret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  s.RedirectURL(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}, p, nil
}

func (s *Service) startOIDC(ctx context.Context, c Connection, st *signinState) (string, error) {
	cfg, _, err := s.oauthConfig(ctx, c)
	if err != nil {
		return "", err
	}
	st.Nonce = randHex(16)
	st.PKCE = oauth2.GenerateVerifier()
	return cfg.AuthCodeURL(st.Key, oauth2.S256ChallengeOption(st.PKCE), oidc.Nonce(st.Nonce)), nil
}

type idClaims struct {
	Email         string `json:"email"`
	EmailVerified *bool  `json:"email_verified"`
}

func (s *Service) finishOIDC(ctx context.Context, c Connection, st signinState, code string) (string, error) {
	if code == "" || len(code) > 4096 {
		return "", errors.New("missing code")
	}
	cfg, p, err := s.oauthConfig(ctx, c)
	if err != nil {
		return "", err
	}
	tok, err := cfg.Exchange(oidc.ClientContext(ctx, s.http), code, oauth2.VerifierOption(st.PKCE))
	if err != nil {
		return "", fmt.Errorf("code exchange: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return "", errors.New("no id_token in the token response")
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: c.OIDCClientID, Now: s.now}).Verify(ctx, raw)
	if err != nil {
		return "", fmt.Errorf("id_token: %w", err)
	}
	if idt.Nonce == "" || idt.Nonce != st.Nonce {
		return "", errors.New("id_token nonce does not match")
	}
	var cl idClaims
	if err := idt.Claims(&cl); err != nil {
		return "", fmt.Errorf("id_token claims: %w", err)
	}
	if cl.EmailVerified != nil && !*cl.EmailVerified {
		return "", errors.New("the identity provider says the email is not verified")
	}
	email, ok := teams.NormalEmail(cl.Email)
	if !ok {
		return "", errors.New("the id_token has no usable email")
	}
	return email, nil
}

// CheckIssuer discovers an OIDC issuer, so a typo is caught when the
// connection is saved rather than at the first sign-in.
func (s *Service) CheckIssuer(ctx context.Context, issuer string) error {
	_, err := s.provider(ctx, issuer)
	return err
}
