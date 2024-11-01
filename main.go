package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/auth0/go-jwt-middleware/v2/validator"
	"golang.org/x/oauth2"
	jwt2 "gopkg.in/go-jose/go-jose.v2/jwt"

	"github.com/dioad/net/http/authz/jwt"
	"github.com/dioad/net/oidc"
	"github.com/dioad/net/oidc/flyio"
)

type DebugStruct struct {
	DecodedToken     interface{}                `json:",omitempty"`
	Errors           []string                   `json:",omitempty"`
	RegisteredClaims validator.RegisteredClaims `json:",omitempty"`
	Claims           jwt2.Claims                `json:",omitempty"`
	AccessToken      string                     `json:",omitempty"`
	Expiry           time.Time                  `json:",omitempty"`
}

func FlyValidator() (jwt.TokenValidator, error) {
	config := []oidc.ValidatorConfig{
		{
			EndpointConfig: oidc.EndpointConfig{
				URL: "https://oidc.fly.io/pat-downey",
			},
			Audiences: []string{"https://fly.io/pat-downey"},
		},
	}

	customClaims := func() validator.CustomClaims { return &oidc.IntrospectionResponse{} }

	v, err := oidc.NewMultiValidatorFromConfig(config, validator.WithCustomClaims(customClaims))
	if err != nil {
		return nil, fmt.Errorf("error creating validator: %w", err)
	}

	return v, nil
}

func FlyDebug() http.HandlerFunc {
	v, err := FlyValidator()
	if err != nil {
		slog.Error("error creating validator", "error", err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		var token *oauth2.Token
		tokenSource := flyio.NewTokenSource()

		o := DebugStruct{}

		token, err = tokenSource.Token()

		o.AccessToken = token.AccessToken
		o.Expiry = token.Expiry

		if err != nil {
			slog.Error("error getting token", "error", err)
			o.Errors = append(o.Errors, fmt.Errorf("error getting token: %w", err).Error())
		}

		if token != nil {
			jwtToken, err := jwt2.ParseSigned(token.AccessToken)
			if err != nil {
				slog.Error("error parsing token", "error", err, "token", token.AccessToken)
				o.Errors = append(o.Errors, fmt.Errorf("error parsing token: %w: %s", err, token.AccessToken).Error())
			} else {
				claims := jwt2.Claims{}
				err = jwtToken.UnsafeClaimsWithoutVerification(&claims)
				if err != nil {
					slog.Error("error decoding token", "error", err, "token", token.AccessToken)
					o.Errors = append(o.Errors, fmt.Errorf("error decoding token: %w: %s", err, token.AccessToken).Error())
				} else {
					o.Claims = claims
				}
			}

			validatedResponse, err := v.ValidateToken(r.Context(), token.AccessToken)
			if err != nil {
				slog.Error("error validating token", "error", err, "token", token.AccessToken)
				o.Errors = append(o.Errors, fmt.Errorf("error validating token: %w: %s", err, token.AccessToken).Error())
			} else {
				rc, _, err := oidc.ExtractClaims[*oidc.IntrospectionResponse](validatedResponse)
				if err != nil {
					slog.Error("error extracting claims", "error", err, "token", token.AccessToken)
					o.Errors = append(o.Errors, fmt.Errorf("error extracting claims: %w", err).Error())
				} else {
					o.RegisteredClaims = rc
				}
			}
		}

		outputBytes, _ := json.Marshal(o)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(outputBytes)
	}
}

func main() {

	server := http.Server{}

	m := http.NewServeMux()
	m.HandleFunc("GET /debug/fly-token", FlyDebug())

	server.Handler = m
	server.Addr = ":8080"
	err := server.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		slog.Error("error calling ListenAndServe", "error", err)
	}
}
