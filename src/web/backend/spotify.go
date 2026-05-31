package backend

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── URL parsing ──────────────────────────────────────────────────────────────

var spotifyPlaylistURLRe = regexp.MustCompile(
	`^https?://open\.spotify\.com/(?:[a-z]{2}-[a-z]{2}/|[a-z]{2}/)?playlist/([a-zA-Z0-9]+)`,
)

func extractSpotifyID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if idx := strings.IndexByte(raw, '?'); idx != -1 {
		raw = raw[:idx]
	}
	m := spotifyPlaylistURLRe.FindStringSubmatch(raw)
	if len(m) < 2 {
		return "", fmt.Errorf("not a valid Spotify playlist URL")
	}
	return m[1], nil
}

// ── TOTP authentication ─────────────────────────────────────────────────────
//
// Spotify's web player authenticates with a TOTP token derived from a secret
// that rotates with client updates. We fetch the latest secret from a
// community-maintained repo, falling back to a hardcoded snapshot.

var (
	fallbackTOTPVersion = 61
	fallbackTOTPSecret  = []byte{
		44, 55, 47, 42, 70, 40, 34, 114, 76, 74,
		50, 111, 120, 97, 75, 76, 94, 102, 43, 69,
		49, 120, 118, 80, 64, 78,
	}
)

var (
	secretCacheMu     sync.Mutex
	cachedVersion     int
	cachedSecret      []byte
	secretCacheExpiry time.Time
)

const secretCacheTTL = 15 * time.Minute

// getSpotifyTOTPSecret returns the current TOTP version and secret bytes.
// Fetches from the remote secrets repo, cached for 15 minutes, with a
// hardcoded fallback if the remote is unreachable.
func getSpotifyTOTPSecret() (int, []byte) {
	secretCacheMu.Lock()
	defer secretCacheMu.Unlock()

	if cachedSecret != nil && time.Now().Before(secretCacheExpiry) {
		return cachedVersion, cachedSecret
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://code.thetadev.de/ThetaDev/spotify-secrets/raw/branch/main/secrets/secretDict.json")
	if err != nil {
		slog.Debug("spotify: failed to fetch remote secrets, using fallback", "err", err)
		return fallbackTOTPVersion, fallbackTOTPSecret
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Debug("spotify: remote secrets returned non-200, using fallback", "status", resp.StatusCode)
		return fallbackTOTPVersion, fallbackTOTPSecret
	}

	var secrets map[string][]int
	if err := json.NewDecoder(resp.Body).Decode(&secrets); err != nil {
		slog.Debug("spotify: failed to parse remote secrets, using fallback", "err", err)
		return fallbackTOTPVersion, fallbackTOTPSecret
	}

	// Find the highest version key
	maxVer := -1
	var maxKey string
	for k := range secrets {
		v, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		if v > maxVer {
			maxVer = v
			maxKey = k
		}
	}
	if maxVer < 0 {
		return fallbackTOTPVersion, fallbackTOTPSecret
	}

	raw := secrets[maxKey]
	secret := make([]byte, len(raw))
	for i, v := range raw {
		secret[i] = byte(v)
	}

	cachedVersion = maxVer
	cachedSecret = secret
	secretCacheExpiry = time.Now().Add(secretCacheTTL)
	slog.Debug("spotify: fetched remote TOTP secret", "version", maxVer, "len", len(secret))
	return maxVer, secret
}

// generateSpotifyTOTP produces a 6-digit TOTP code and version number.
// The secret bytes are XOR-transformed, joined as decimal strings, then
// base32-encoded before being fed into standard RFC 6238 TOTP.
func generateSpotifyTOTP() (string, int) {
	version, secretBytes := getSpotifyTOTPSecret()

	transformed := make([]int, len(secretBytes))
	for i, b := range secretBytes {
		transformed[i] = int(b) ^ ((i % 33) + 9)
	}

	var joined strings.Builder
	for _, n := range transformed {
		joined.WriteString(strconv.Itoa(n))
	}

	totpSecret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(joined.String()))
	return computeTOTP(totpSecret), version
}

// computeTOTP implements RFC 6238 with a 30-second period and 6-digit output.
func computeTOTP(base32Secret string) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(base32Secret)
	if err != nil {
		key, _ = base32.StdEncoding.DecodeString(base32Secret)
	}

	counter := uint64(time.Now().Unix() / 30)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0f
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1000000)
}

// ── Session management ──────────────────────────────────────────────────────
//
// A spotifySession caches all auth state (cookies, tokens, GraphQL hash) with
// a 10-minute TTL. On failure the caller invalidates and retries once.

const spotifyUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"

type spotifySession struct {
	mu            sync.Mutex
	client        *http.Client
	clientVersion string
	deviceID      string
	accessToken   string
	clientID      string
	clientToken   string
	playlistHash  string
	expiresAt     time.Time
}

var spSession = &spotifySession{}

func (s *spotifySession) invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expiresAt = time.Time{}
}

// ensure initializes or reuses the session. Four-step flow:
// visitHome → fetchAccessToken (TOTP) → fetchClientToken → extractPlaylistHash.
func (s *spotifySession) ensure() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if time.Now().Before(s.expiresAt) {
		return nil
	}

	slog.Info("spotify: initializing new session")

	jar, _ := cookiejar.New(nil)
	s.client = &http.Client{Jar: jar, Timeout: 30 * time.Second}

	jsPack, err := s.visitHome()
	if err != nil {
		return fmt.Errorf("spotify session: visit home failed: %w", err)
	}
	if err := s.fetchAccessToken(); err != nil {
		return fmt.Errorf("spotify session: access token failed: %w", err)
	}
	if err := s.fetchClientToken(); err != nil {
		return fmt.Errorf("spotify session: client token failed: %w", err)
	}
	if err := s.extractPlaylistHash(jsPack); err != nil {
		return fmt.Errorf("spotify session: hash extraction failed: %w", err)
	}

	s.expiresAt = time.Now().Add(10 * time.Minute)

	hashPreview := s.playlistHash
	if len(hashPreview) > 16 {
		hashPreview = hashPreview[:16] + "..."
	}
	slog.Info("spotify: session ready",
		"version", s.clientVersion,
		"hash", hashPreview,
	)
	return nil
}

// visitHome fetches open.spotify.com to establish cookies and extract the
// client version, device ID (sp_t cookie), and web-player JS bundle URL.
func (s *spotifySession) visitHome() (string, error) {
	req, err := http.NewRequest("GET", "https://open.spotify.com", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", spotifyUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", err
	}
	html := string(body)

	// Client version from base64-encoded appServerConfig
	const cfgStart = `<script id="appServerConfig" type="text/plain">`
	const cfgEnd = `</script>`
	startIdx := strings.Index(html, cfgStart)
	if startIdx < 0 {
		return "", fmt.Errorf("appServerConfig not found in page")
	}
	startIdx += len(cfgStart)
	endIdx := strings.Index(html[startIdx:], cfgEnd)
	if endIdx < 0 {
		return "", fmt.Errorf("appServerConfig closing tag not found")
	}

	cfgJSON, err := base64.StdEncoding.DecodeString(strings.TrimSpace(html[startIdx : startIdx+endIdx]))
	if err != nil {
		cfgJSON, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(html[startIdx : startIdx+endIdx]))
		if err != nil {
			return "", fmt.Errorf("failed to decode appServerConfig: %w", err)
		}
	}

	var serverCfg struct {
		ClientVersion string `json:"clientVersion"`
	}
	if err := json.Unmarshal(cfgJSON, &serverCfg); err != nil {
		return "", fmt.Errorf("failed to parse appServerConfig: %w", err)
	}
	s.clientVersion = serverCfg.ClientVersion

	// Device ID from sp_t cookie
	u, _ := url.Parse("https://open.spotify.com")
	for _, c := range s.client.Jar.Cookies(u) {
		if c.Name == "sp_t" {
			s.deviceID = c.Value
			break
		}
	}

	// Find the web-player JS bundle
	var jsPack string
	for _, link := range extractJSLinks(html) {
		if strings.Contains(link, "web-player/web-player") && strings.HasSuffix(link, ".js") {
			jsPack = link
			break
		}
	}
	if jsPack == "" {
		return "", fmt.Errorf("web-player JS pack not found in page")
	}

	return jsPack, nil
}

// fetchAccessToken obtains a bearer token via TOTP.
func (s *spotifySession) fetchAccessToken() error {
	totp, version := generateSpotifyTOTP()

	tokenURL := "https://open.spotify.com/api/token?" + url.Values{
		"reason":      {"init"},
		"productType": {"web-player"},
		"totp":        {totp},
		"totpVer":     {strconv.Itoa(version)},
		"totpServer":  {totp},
	}.Encode()

	req, err := http.NewRequest("GET", tokenURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", spotifyUA)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://open.spotify.com/")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(b))
	}

	var tok struct {
		AccessToken string `json:"accessToken"`
		ClientID    string `json:"clientId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}
	if tok.AccessToken == "" {
		return fmt.Errorf("received empty access token")
	}

	s.accessToken = tok.AccessToken
	s.clientID = tok.ClientID
	return nil
}

// fetchClientToken obtains a client token from clienttoken.spotify.com.
func (s *spotifySession) fetchClientToken() error {
	payload, err := json.Marshal(map[string]any{
		"client_data": map[string]any{
			"client_version": s.clientVersion,
			"client_id":      s.clientID,
			"js_sdk_data": map[string]any{
				"device_brand": "unknown",
				"device_model": "unknown",
				"os":           "windows",
				"os_version":   "NT 10.0",
				"device_id":    s.deviceID,
				"device_type":  "computer",
			},
		},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://clienttoken.spotify.com/v1/clienttoken", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", spotifyUA)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("client token endpoint returned %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		ResponseType string `json:"response_type"`
		GrantedToken struct {
			Token string `json:"token"`
		} `json:"granted_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse client token response: %w", err)
	}
	if result.ResponseType != "RESPONSE_GRANTED_TOKEN_RESPONSE" {
		return fmt.Errorf("unexpected client token response type: %s", result.ResponseType)
	}
	if result.GrantedToken.Token == "" {
		return fmt.Errorf("received empty client token")
	}

	s.clientToken = result.GrantedToken.Token
	return nil
}

// extractPlaylistHash fetches the web-player JS bundle and its webpack chunks
// to find the SHA256 hash for the "fetchPlaylist" persisted GraphQL query.
func (s *spotifySession) extractPlaylistHash(jsPackURL string) error {
	req, err := http.NewRequest("GET", jsPackURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", spotifyUA)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	packBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return err
	}
	jsContent := string(packBody)

	// Check the main pack first
	if hash := findPartHash(jsContent, "fetchPlaylist"); hash != "" {
		s.playlistHash = hash
		return nil
	}

	// Fall back to scanning webpack chunks
	nameMap, hashMap, err := extractMappings(jsContent)
	if err != nil {
		return fmt.Errorf("failed to extract chunk mappings: %w", err)
	}

	chunkFiles := combineChunks(nameMap, hashMap)
	slog.Debug("spotify: scanning JS chunks for hash", "count", len(chunkFiles))

	type chunkResult struct {
		content string
		err     error
	}

	sem := make(chan struct{}, 10)
	results := make(chan chunkResult, len(chunkFiles))

	for _, file := range chunkFiles {
		chunkURL := "https://open.spotifycdn.com/cdn/build/web-player/" + file
		go func(u string) {
			sem <- struct{}{}
			defer func() { <-sem }()

			r, err := http.NewRequest("GET", u, nil)
			if err != nil {
				results <- chunkResult{err: err}
				return
			}
			r.Header.Set("User-Agent", spotifyUA)

			resp, err := s.client.Do(r)
			if err != nil {
				results <- chunkResult{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()

			b, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
			if err != nil {
				results <- chunkResult{err: err}
				return
			}
			results <- chunkResult{content: string(b)}
		}(chunkURL)
	}

	for i := 0; i < len(chunkFiles); i++ {
		r := <-results
		if r.err != nil {
			continue
		}
		if hash := findPartHash(r.content, "fetchPlaylist"); hash != "" {
			s.playlistHash = hash
			for j := i + 1; j < len(chunkFiles); j++ {
				<-results
			}
			return nil
		}
	}

	return fmt.Errorf("fetchPlaylist hash not found in any JS bundle")
}

// ── Partner API ─────────────────────────────────────────────────────────────

type spotifyImageSource struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type partnerPlaylistResp struct {
	Data struct {
		PlaylistV2 struct {
			Name    string         `json:"name"`
			Images  struct {
				Items []struct {
					Sources []spotifyImageSource `json:"sources"`
				} `json:"items"`
			} `json:"images"`
			Content partnerContent `json:"content"`
		} `json:"playlistV2"`
	} `json:"data"`
}

type partnerContent struct {
	TotalCount int           `json:"totalCount"`
	Items      []partnerItem `json:"items"`
}

type partnerItem struct {
	ItemV2 struct {
		Data partnerTrackData `json:"data"`
	} `json:"itemV2"`
}

type partnerTrackData struct {
	Typename string `json:"__typename"`
	Name     string `json:"name"`
	Artists  struct {
		Items []struct {
			Profile struct {
				Name string `json:"name"`
			} `json:"profile"`
		} `json:"items"`
	} `json:"artists"`
	AlbumOfTrack struct {
		Name     string `json:"name"`
		CoverArt struct {
			Sources []spotifyImageSource `json:"sources"`
		} `json:"coverArt"`
	} `json:"albumOfTrack"`
}

// ── Playlist fetching ───────────────────────────────────────────────────────

// fetchSpotifyPlaylist fetches a public Spotify playlist via the internal
// partner API (api-partner.spotify.com). Retries once with a fresh session
// on failure. Returns playlist name, artwork URL, and normalized tracks.
func fetchSpotifyPlaylist(playlistURL string) (string, string, []PlaylistTrack, error) {
	id, err := extractSpotifyID(playlistURL)
	if err != nil {
		return "", "", nil, err
	}

	name, artwork, tracks, err := fetchPlaylistByID(id)
	if err != nil {
		slog.Debug("spotify: retrying with fresh session", "err", err)
		spSession.invalidate()
		name, artwork, tracks, err = fetchPlaylistByID(id)
	}
	if err != nil {
		return "", "", nil, err
	}
	if len(tracks) == 0 {
		return "", "", nil, fmt.Errorf("spotify: playlist %q has no tracks", id)
	}

	slog.Info("spotify: fetched playlist", "name", name, "tracks", len(tracks))
	return name, artwork, tracks, nil
}

func fetchPlaylistByID(id string) (string, string, []PlaylistTrack, error) {
	if err := spSession.ensure(); err != nil {
		return "", "", nil, err
	}

	const pageSize = 343

	resp, err := queryPartnerAPI(id, 0, pageSize)
	if err != nil {
		return "", "", nil, err
	}

	pl := resp.Data.PlaylistV2
	name := pl.Name
	if name == "" {
		name = "Spotify Playlist"
	}

	artworkURL := ""
	if len(pl.Images.Items) > 0 && len(pl.Images.Items[0].Sources) > 0 {
		artworkURL = pickBestSource(pl.Images.Items[0].Sources, 300)
	}

	tracks := extractTracks(pl.Content.Items)

	// Paginate
	for offset := pageSize; offset < pl.Content.TotalCount; offset += pageSize {
		page, err := queryPartnerAPI(id, offset, pageSize)
		if err != nil {
			slog.Warn("spotify: pagination failed, returning partial results", "offset", offset, "err", err)
			break
		}
		tracks = append(tracks, extractTracks(page.Data.PlaylistV2.Content.Items)...)
	}

	return name, artworkURL, tracks, nil
}

// queryPartnerAPI sends a persisted GraphQL query to api-partner.spotify.com.
func queryPartnerAPI(playlistID string, offset, limit int) (*partnerPlaylistResp, error) {
	variables, _ := json.Marshal(map[string]any{
		"uri":                       "spotify:playlist:" + playlistID,
		"offset":                    offset,
		"limit":                     limit,
		"enableWatchFeedEntrypoint": false,
	})
	extensions, _ := json.Marshal(map[string]any{
		"persistedQuery": map[string]any{
			"version":    1,
			"sha256Hash": spSession.playlistHash,
		},
	})

	reqURL := "https://api-partner.spotify.com/pathfinder/v1/query?" + url.Values{
		"operationName": {"fetchPlaylist"},
		"variables":     {string(variables)},
		"extensions":    {string(extensions)},
	}.Encode()

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+spSession.accessToken)
	req.Header.Set("Client-Token", spSession.clientToken)
	req.Header.Set("Spotify-App-Version", spSession.clientVersion)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("User-Agent", spotifyUA)

	resp, err := spSession.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("partner API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("partner API returned %d: %s", resp.StatusCode, string(b))
	}

	var result partnerPlaylistResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse partner API response: %w", err)
	}
	return &result, nil
}

// ── Track extraction helpers ────────────────────────────────────────────────

func extractTracks(items []partnerItem) []PlaylistTrack {
	tracks := make([]PlaylistTrack, 0, len(items))
	for _, item := range items {
		t := item.ItemV2.Data
		if t.Name == "" {
			continue
		}

		artists := make([]string, 0, len(t.Artists.Items))
		for _, a := range t.Artists.Items {
			if a.Profile.Name != "" {
				artists = append(artists, a.Profile.Name)
			}
		}

		fullArtist := strings.Join(artists, ", ")
		mainArtist := fullArtist
		if len(artists) > 0 {
			mainArtist = artists[0]
		}

		coverURL := ""
		if len(t.AlbumOfTrack.CoverArt.Sources) > 0 {
			coverURL = pickBestSource(t.AlbumOfTrack.CoverArt.Sources, 300)
		}

		tracks = append(tracks, PlaylistTrack{
			Title:      t.Name,
			Artist:     fullArtist,
			MainArtist: mainArtist,
			Album:      t.AlbumOfTrack.Name,
			CoverURL:   coverURL,
		})
	}
	return tracks
}

// pickBestSource returns the image URL closest to targetSize pixels.
func pickBestSource(sources []spotifyImageSource, targetSize int) string {
	if len(sources) == 0 {
		return ""
	}
	best := sources[0].URL
	bestDiff := abs(sources[0].Height - targetSize)
	for _, s := range sources[1:] {
		if s.Height == 0 {
			continue
		}
		if diff := abs(s.Height - targetSize); diff < bestDiff {
			best = s.URL
			bestDiff = diff
		}
	}
	return best
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ── JS bundle parsing ───────────────────────────────────────────────────────
//
// Spotify's web player uses webpack. The persisted GraphQL query hashes are
// embedded in JS chunks. These helpers extract <script> URLs from the home
// page HTML, parse webpack chunk mappings, and search for operation hashes.

var scriptSrcRe = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)

func extractJSLinks(html string) []string {
	matches := scriptSrcRe.FindAllStringSubmatch(html, -1)
	links := make([]string, 0, len(matches))
	for _, m := range matches {
		if strings.HasSuffix(m[1], ".js") {
			links = append(links, m[1])
		}
	}
	return links
}

var (
	jsObjectMapRe = regexp.MustCompile(`\{\d+:"[^"]+"(?:,\d+:"[^"]+")*\}`)
	jsKVRe        = regexp.MustCompile(`(\d+):"([^"]+)"`)
)

// extractMappings pulls webpack chunk name/hash mappings from the main JS pack.
func extractMappings(jsCode string) (nameMap, hashMap map[int]string, err error) {
	matches := jsObjectMapRe.FindAllString(jsCode, -1)
	if len(matches) < 5 {
		return nil, nil, fmt.Errorf("found only %d object maps, need at least 5", len(matches))
	}
	nameMap = parseJSObjectMap(matches[4])
	hashMap = parseJSObjectMap(matches[3])
	return nameMap, hashMap, nil
}

func parseJSObjectMap(s string) map[int]string {
	m := make(map[int]string)
	for _, match := range jsKVRe.FindAllStringSubmatch(s, -1) {
		k, _ := strconv.Atoi(match[1])
		m[k] = match[2]
	}
	return m
}

func combineChunks(nameMap, hashMap map[int]string) []string {
	chunks := make([]string, 0, len(nameMap))
	for key, name := range nameMap {
		if hash, ok := hashMap[key]; ok {
			chunks = append(chunks, name+"."+hash+".js")
		}
	}
	return chunks
}

// findPartHash searches JS content for a persisted-query hash by operation name.
func findPartHash(jsContent, name string) string {
	needle := `"` + name + `","query","`
	idx := strings.Index(jsContent, needle)
	if idx < 0 {
		needle = `"` + name + `","mutation","`
		idx = strings.Index(jsContent, needle)
	}
	if idx < 0 {
		return ""
	}
	rest := jsContent[idx+len(needle):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}
