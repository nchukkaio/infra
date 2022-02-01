package authn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/infrahq/infra/internal"
	"github.com/infrahq/infra/internal/registry/models"
	"golang.org/x/oauth2"
	"gopkg.in/square/go-jose.v2/jwt"
)

// UserInfo captures the fields from a user-info response that we care about
type UserInfo struct {
	Email  string   `json:"email"`
	Groups []string `json:"groups"`
}

type OIDC interface {
	ExchangeAuthCodeForProviderTokens(code string) (accessToken, refreshToken string, accessTokenExpiry time.Time, email string, err error)
	RefreshAccessToken(providerToken *models.ProviderToken) (accessToken string, expiry *time.Time, err error)
	GetUserInfo(providerToken *models.ProviderToken) (*UserInfo, error)
}

type oidcImplementation struct {
	Domain       string
	ClientID     string
	ClientSecret string
}

func NewOIDC(domain, clientID, clientSecret string) OIDC {
	return &oidcImplementation{
		Domain:       domain,
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
}

// clientConfig returns the OAuth client configuration needed to interact with an identity provider
func (o *oidcImplementation) clientConfig(ctx context.Context) (*oauth2.Config, *oidc.Provider, error) {
	// TODO: #834 we should be caching this information locally
	provider, err := oidc.NewProvider(ctx, fmt.Sprintf("https://%s", o.Domain))
	if err != nil {
		return nil, nil, fmt.Errorf("get provider openid info: %w", err)
	}

	conf := &oauth2.Config{
		ClientID:     o.ClientID,
		ClientSecret: o.ClientSecret,
		RedirectURL:  "http://localhost:8301",
		Scopes:       []string{oidc.ScopeOpenID, "email", "groups", oidc.ScopeOfflineAccess},
		Endpoint:     provider.Endpoint(),
	}

	return conf, provider, nil
}

// tokenSource is used to call an identity provider with the specified provider tokens
func (o *oidcImplementation) tokenSource(providerTokens *models.ProviderToken) (oauth2.TokenSource, error) {
	ctx := context.Background()
	conf, _, err := o.clientConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("call idp with tokens: %w", err)
	}

	userToken := &oauth2.Token{
		AccessToken:  string(providerTokens.AccessToken),
		RefreshToken: string(providerTokens.RefreshToken),
		Expiry:       providerTokens.ExpiresAt,
	}

	return conf.TokenSource(ctx, userToken), nil
}

func (o *oidcImplementation) ExchangeAuthCodeForProviderTokens(code string) (rawAccessToken, rawRefreshToken string, accessTokenExpiry time.Time, email string, err error) {
	ctx := context.Background()
	conf, provider, err := o.clientConfig(ctx)
	if err != nil {
		return "", "", time.Time{}, "", fmt.Errorf("client exchange code: %w", err)
	}

	exchanged, err := conf.Exchange(ctx, code)
	if err != nil {
		return "", "", time.Time{}, "", fmt.Errorf("code exchange: %w", err)
	}

	rawAccessToken, ok := exchanged.Extra("access_token").(string)
	if !ok {
		return "", "", time.Time{}, "", errors.New("could not extract access token from oauth2")
	}

	rawRefreshToken, ok = exchanged.Extra("refresh_token").(string)
	if !ok {
		return "", "", time.Time{}, "", errors.New("could not extract refresh token from oauth2")
	}

	rawIDToken, ok := exchanged.Extra("id_token").(string)
	if !ok {
		return "", "", time.Time{}, "", errors.New("could not extract id_token from oauth2 token")
	}

	exp, err := getAccessTokenExpiry(rawAccessToken)
	if err != nil {
		return "", "", time.Time{}, "", fmt.Errorf("get exp: %w", err)
	}

	// we get sensitive claims from the ID token, must validate them
	verifier := provider.Verifier(&oidc.Config{ClientID: o.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", "", time.Time{}, "", fmt.Errorf("validate id token: %w", err)
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return "", "", time.Time{}, "", fmt.Errorf("id cliams: %w", err)
	}

	return rawAccessToken, rawRefreshToken, exp, claims.Email, nil
}

// RefreshAccessToken uses the refresh token to get a new access token if it is expired
func (o *oidcImplementation) RefreshAccessToken(providerTokens *models.ProviderToken) (accessToken string, expiry *time.Time, err error) {
	tokenSource, err := o.tokenSource(providerTokens)
	if err != nil {
		return "", nil, fmt.Errorf("ref token source: %w", err)
	}

	newToken, err := tokenSource.Token() // this refreshes token if needed
	if err != nil {
		return "", nil, fmt.Errorf("refresh user token: %w", err)
	}

	return newToken.AccessToken, &newToken.Expiry, nil
}

// GetUserInfo uses a provider token to get the current information about a user,
// make sure an access token is valid (not expired) before using this
func (o *oidcImplementation) GetUserInfo(providerTokens *models.ProviderToken) (*UserInfo, error) {
	ctx := context.Background()
	tokenSource, err := o.tokenSource(providerTokens)
	if err != nil {
		return nil, fmt.Errorf("info token source: %w", err)
	}

	userInfoEndpoint := fmt.Sprintf("https://%s/oauth2/v1/userinfo", o.Domain)

	client := oauth2.NewClient(ctx, tokenSource)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("userinfo request %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+string(providerTokens.AccessToken))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user info response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// access token has been revoked, user is no longer valid
		return nil, internal.ErrForbidden
	}

	info := &UserInfo{}

	err = json.NewDecoder(resp.Body).Decode(info)
	if err != nil {
		return nil, fmt.Errorf("decode user info response: %w", err)
	}

	return info, nil
}

func getAccessTokenExpiry(rawAccessToken string) (time.Time, error) {
	accessToken, err := jwt.ParseSigned(rawAccessToken)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse acc: %w", err)
	}

	accClaims := &jwt.Claims{}
	// as long as we are only getting the expiry for the access token claims here we dont need to validate them
	err = accessToken.UnsafeClaimsWithoutVerification(accClaims)
	if err != nil {
		return time.Time{}, fmt.Errorf("acc token exp claim: %w", err)
	}
	return accClaims.Expiry.Time(), nil
}