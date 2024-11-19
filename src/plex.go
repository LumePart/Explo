package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"fmt"
	"net/http"

	"github.com/LukeHagar/plexgo"
	"github.com/LukeHagar/plexgo/models/operations"
)

type Auth struct {
	AuthToken         string `json:"authToken"`
}

func parsePlexResp[T any](body io.ReadCloser, target *T) error {
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("error unmarshaling response body: %w", err)
	}
	return nil
}

func callPlex[T any](ctx context.Context, apiCall func(ctx context.Context) (*http.Response, error), target *T) error { // generic function to parse multiple struct types
	res, err := apiCall(ctx)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}
	if res == nil || res.Body == nil {
		return fmt.Errorf("empty response from API")
	}
	return parsePlexResp(res.Body, target)
}

func (cfg *Credentials) getPlexAuth(ctx context.Context, client *plexgo.PlexAPI) {
	request := operations.PostUsersSignInDataRequest{
		RequestBody: &operations.PostUsersSignInDataRequestBody{
			Login:    cfg.User,
			Password: cfg.Password,
		},
	}

	auth := &Auth{}
	err := callPlex(ctx, func(ctx context.Context) (*http.Response, error) {
		resp, err := client.Authentication.PostUsersSignInData(ctx, request)
		if err != nil {
			return nil, err
		}
		return resp.RawResponse, nil
	}, auth)

	if err != nil {
		log.Fatalf("Failed to authenticate: %v", err)
	}

	cfg.APIKey = auth.AuthToken
}