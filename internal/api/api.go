package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client talks to the NAS-Stack services.
type Client struct {
	TorrentsURL  string
	StatsURL     string
	SchedulerURL string
	MusicURL     string
	SearchURL    string
	AdminToken   string
	HTTP         *http.Client
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// New builds a client with sane defaults and reads ADMIN_TOKEN from the
// environment or from the nas-stack .env file.
func New() *Client {
	return &Client{
		TorrentsURL:  env("NASDASH_TORRENTS_URL", "http://localhost:8081"),
		StatsURL:     env("NASDASH_STATS_URL", "http://localhost:8082"),
		SchedulerURL: env("NASDASH_SCHEDULER_URL", "http://localhost:8084"),
		MusicURL:     env("NASDASH_MUSIC_URL", "http://localhost:8080/music"),
		SearchURL:    env("NASDASH_SEARCH_URL", "http://localhost:8088"),
		AdminToken:   adminToken(),
		HTTP:         &http.Client{Timeout: 20 * time.Second},
	}
}

// adminToken resolves the scheduler token without ever printing it.
func adminToken() string {
	if v := strings.TrimSpace(os.Getenv("ADMIN_TOKEN")); v != "" {
		return v
	}
	paths := []string{
		os.ExpandEnv("$HOME/projects/nas-stack/.env"),
		"/volume1/docker/nas-stack/.env",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ADMIN_TOKEN=") {
				return strings.Trim(strings.TrimPrefix(line, "ADMIN_TOKEN="), `"' `)
			}
		}
	}
	return ""
}

func (c *Client) do(ctx context.Context, method, rawurl string, body any, headers map[string]string, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawurl, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("%s %s: HTTP %d %s", method, rawurl, resp.StatusCode, msg)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s: %w", rawurl, err)
		}
	}
	return nil
}

// ---- torrents ----

type Torrent struct {
	ID             string  `json:"id"`
	Progress       float64 `json:"progress"`
	Size           int64   `json:"size"`
	SizeDownloaded int64   `json:"size_downloaded"`
	SpeedDownload  int64   `json:"speed_download"`
	SpeedUpload    int64   `json:"speed_upload"`
	Status         string  `json:"status"`
	Additional     struct {
		Detail struct {
			URI         string `json:"uri"`
			Destination string `json:"destination"`
			CreateTime  int64  `json:"create_time"`
		} `json:"detail"`
	} `json:"additional"`
}

func (t Torrent) Name() string {
	v := t.Additional.Detail.URI
	if v == "" {
		return t.ID
	}
	if strings.HasPrefix(v, "magnet:") {
		if u, err := url.Parse(v); err == nil {
			if dn := u.Query().Get("dn"); dn != "" {
				return dn
			}
		}
	}
	if u, err := url.Parse(v); err == nil {
		if dn := u.Query().Get("dn"); dn != "" {
			return dn
		}
		if base := u.Path; base != "" {
			if i := strings.LastIndex(base, "/"); i >= 0 && i+1 < len(base) {
				return base[i+1:]
			}
		}
	}
	return t.ID
}

func (c *Client) Torrents(ctx context.Context) ([]Torrent, error) {
	var out struct {
		Data struct {
			Task []Torrent `json:"task"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, c.TorrentsURL+"/list", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Data.Task, nil
}

func (c *Client) AddTorrent(ctx context.Context, magnet, destination string) error {
	body := map[string]string{"magnet": magnet}
	if destination != "" {
		body["destination"] = destination
	}
	return c.do(ctx, http.MethodPost, c.TorrentsURL+"/add", body, nil, nil)
}

func (c *Client) DeleteTorrent(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, c.TorrentsURL+"/"+url.PathEscape(id), nil, nil, nil)
}

func (c *Client) TorrentAction(ctx context.Context, action, id string) error {
	return c.do(ctx, http.MethodPost, c.TorrentsURL+"/"+action+"/"+url.PathEscape(id), nil, nil, nil)
}

// ---- search ----

type SearchHit struct {
	Title   string
	Size    string
	Seeders string
	Link    string
}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if t != "" {
					return t
				}
			case float64:
				return strconv.FormatFloat(t, 'f', 0, 64)
			}
		}
	}
	return ""
}

// Search queries the search-service; the upstream schema varies by indexer, so
// fields are picked defensively.
func (c *Client) Search(ctx context.Context, q string) ([]SearchHit, error) {
	var out struct {
		Results []map[string]any `json:"results"`
	}
	u := c.SearchURL + "/search?q=" + url.QueryEscape(q) + "&limit=50"
	// The search-service fans out to indexers; give it room (up to ~65s there).
	slow := *c
	slow.HTTP = &http.Client{Timeout: 85 * time.Second}
	if err := slow.do(ctx, http.MethodGet, u, nil, nil, &out); err != nil {
		return nil, err
	}
	hits := make([]SearchHit, 0, len(out.Results))
	for _, r := range out.Results {
		hits = append(hits, SearchHit{
			Title:   str(r, "title", "name"),
			Size:    str(r, "size", "sizeText", "sizeHuman"),
			Seeders: str(r, "seeders", "seeds"),
			Link:    str(r, "magnet", "magnetUri", "downloadUrl", "url", "link"),
		})
	}
	return hits, nil
}

// ---- stats ----

type Stats struct {
	CPU struct {
		Model string  `json:"model"`
		Usage float64 `json:"usage"`
	} `json:"cpu"`
	RAM struct {
		Percent string `json:"percent"`
		TotalGB string `json:"total_gb"`
		UsedGB  string `json:"used_gb"`
	} `json:"ram"`
	Disks []struct {
		Device  string `json:"device"`
		Mount   string `json:"mount"`
		Total   string `json:"total"`
		Used    string `json:"used"`
		Free    string `json:"free"`
		Percent string `json:"percent"`
		Fstype  string `json:"fstype"`
	} `json:"disks"`
	GPU map[string]any `json:"gpu"`
}

func (c *Client) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	err := c.do(ctx, http.MethodGet, c.StatsURL+"/all", nil, nil, &s)
	return s, err
}

// ---- scheduler ----

type Task struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Status   string `json:"status"`
	LastRun  string `json:"last_run"`
}

func (c *Client) Tasks(ctx context.Context) ([]Task, error) {
	var out []Task
	err := c.do(ctx, http.MethodGet, c.SchedulerURL+"/tasks", nil, nil, &out)
	return out, err
}

func (c *Client) RunTask(ctx context.Context, name string) (string, error) {
	if c.AdminToken == "" {
		return "", fmt.Errorf("ADMIN_TOKEN не найден (env или nas-stack/.env)")
	}
	var out map[string]any
	hdr := map[string]string{"X-Admin-Token": c.AdminToken}
	if err := c.do(ctx, http.MethodPost, c.SchedulerURL+"/run/"+url.PathEscape(name), nil, hdr, &out); err != nil {
		return "", err
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ---- music ----

type MusicStatus struct {
	State    string
	Volume   int
	Elapsed  float64
	Duration float64
	Title    string
	Artist   string
	Album    string
	Err      string
}

func (c *Client) MusicStatus(ctx context.Context) (MusicStatus, error) {
	var m map[string]any
	if err := c.do(ctx, http.MethodGet, c.MusicURL+"/status", nil, nil, &m); err != nil {
		return MusicStatus{}, err
	}
	st := MusicStatus{
		State:  pick(m, "state", "State"),
		Title:  pick(m, "Title", "title"),
		Artist: pick(m, "Artist", "artist"),
		Album:  pick(m, "Album", "album"),
	}
	st.Volume = pickInt(m, "volume", "Volume")
	st.Elapsed = pickFloat(m, "elapsed", "Elapsed")
	st.Duration = pickFloat(m, "duration", "Duration")
	if v := pick(m, "mpd"); v == "down" {
		st.Err = "MPD недоступен"
	}
	return st, nil
}

func (c *Client) MusicAction(ctx context.Context, action string) error {
	return c.do(ctx, http.MethodPost, c.MusicURL+"/"+action, nil, nil, nil)
}

func (c *Client) MusicVolume(ctx context.Context, level int) error {
	return c.do(ctx, http.MethodPost, c.MusicURL+"/volume", map[string]int{"level": level}, nil, nil)
}

func pick(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func pickInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if f, ok := v.(float64); ok {
				return int(f)
			}
		}
	}
	return 0
}

func pickFloat(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if f, ok := v.(float64); ok {
				return f
			}
		}
	}
	return 0
}

// ---- humanize ----

func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func Speed(n int64) string { return Bytes(n) + "/s" }
