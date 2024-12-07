package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type LoginPayload struct {
	User LoginUser `json:"user"`
}

type LoginUser struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type LoginResponse struct {
	User struct {
		AuthToken string `json:"authToken"`
	} `json:"user"`
}

type Libraries struct {
	MediaContainer struct {
		Size      int    `json:"size"`
		AllowSync bool   `json:"allowSync"`
		Title1    string `json:"title1"`
		Library []struct {
			Title 			 string `json:"title"`
			Key              json.Number `json:"key"`
			Location         []struct {
				ID   float64    `json:"id"`
				Path string `json:"path"`
			} `json:"Location"`
		} `json:"Directory"`
	} `json:"MediaContainer"`
}

func (cfg *Credentials) PlexHeader() {
	cfg.Headers = make(map[string]string)

	cfg.Headers["X-Plex-Identifier"] = "explo"
}


func (cfg *Credentials) getPlexAuth() { // Get user token from plex
	payload := LoginPayload{
		User: LoginUser{
			Login:    cfg.User,
			Password: cfg.Password,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Fatalf("failed to marshal payload: %s", err.Error())
	}


	body, err := makeRequest("POST", "https://plex.tv/users/sign_in.json", bytes.NewBuffer(payloadBytes), cfg.Headers)
	
	if err != nil {
		log.Fatalf("failed to make request to plex: %s", err.Error())
	}

	var auth LoginResponse
	err = parseResp(body, &auth)
	if err != nil {
		log.Fatalf("getPlexAuth(): %s", err.Error())
	}

	cfg.APIKey = auth.User.AuthToken
}

func getPlexLibraries(cfg Config) (Libraries, error) {

	params := fmt.Sprintf("/library/sections/?X-Plex-Token=%s", cfg.Creds.APIKey)

	body, err := makeRequest("GET", cfg.URL+params, nil, cfg.Creds.Headers)
	if err != nil {
		return Libraries{}, fmt.Errorf("failed to make request to plex: %s", err.Error())
	}

	var libraries Libraries
	err = json.Unmarshal(body, &libraries)
	if err != nil {
		return Libraries{}, fmt.Errorf("failed to unmarshal response: %s", err.Error())
	}
	return libraries, nil
}

func (cfg *Plex) getPlexLibrary(ctx context.Context, client *plexgo.PlexAPI) error {
	var libraries Libraries
	err := callPlex(ctx, func(ctx context.Context) (*http.Response, error) {
		res, err := client.Library.GetAllLibraries(ctx)
		if err != nil {
			return nil, err
		}
		return res.RawResponse, nil
	}, &libraries)

	if err != nil {
		return fmt.Errorf("failed to fetch libraries: %s", err.Error())
	}
	for _, library := range libraries.MediaContainer.Library {
		if cfg.LibraryName == library.Title {
			key, err := library.Key.Float64()
			if err != nil {
				fmt.Errorf("failed to get library number: %s", err.Error())
			}
			cfg.LibraryID = plexgo.Float64(key)
		}
	}
	return fmt.Errorf("no library named %s found, please check LIBRARY_NAME variable", cfg.LibraryName)
}