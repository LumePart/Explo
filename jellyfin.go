package main

import (
	"bytes"
	"fmt"
	"log"
)

func (cfg *Credentials) JellyfinHeader() {
	cfg.Headers = make(map[string]string)

	cfg.Headers["Authorization"] = fmt.Sprintf("MediaBrowser Token=%s, Client=Explo", cfg.APIKey)
	
}

func jellyfinAddPath(cfg Config)  {

	params := "/Library/VirtualFolders/Paths"
	payload := []byte(fmt.Sprintf(`{
		"Name": "%s",
		"Path": "%s"
		}`, cfg.Jellyfin.LibraryName, cfg.Youtube.DownloadDir))
	fmt.Println(cfg.Creds.Headers)
	body, err := makeRequest("POST", cfg.URL+params,bytes.NewReader(payload),cfg.Creds.Headers)

	if err != nil {
		log.Fatalf("failed to add path: %s", err.Error())
	}
	fmt.Println(string(body))
}