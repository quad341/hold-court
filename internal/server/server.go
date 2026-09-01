// Package server implements Hold Court's web UI: a mutt-style three-pane
// (folders / hold list / reading pane) view over the hold feed, driven by
// vim keybindings (see keymap.go), plus the JSON APIs behind the rulings-out
// and read-state actions.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/quad341/hold-court/internal/feed"
	"github.com/quad341/hold-court/internal/ruling"
	"github.com/quad341/hold-court/internal/store"
)

//go:embed static
var staticFS embed.FS

//go:embed templates
var templatesFS embed.FS

// Config configures a Hold Court server instance.
type Config struct {
	// FeedDir holds the hold JSON documents (DESIGN.md feed contract v0).
	FeedDir string
	// RulingsDir is where rulings and their hook results are read/written.
	RulingsDir string
	// Store persists per-user read state. Required.
	Store *store.Store
	// OnRuling is the optional hook command run, with the ruling as JSON on
	// stdin, after a ruling is written. Empty means no hook.
	OnRuling []string
	// User identifies the current maintainer for read state and ruled_by.
	// Defaults to "operator" when empty.
	User string
}

// feedRescanInterval bounds how stale the feed cache can be when fsnotify
// doesn't fire (or can't attach) — see feedcache.go.
const feedRescanInterval = 5 * time.Second

type server struct {
	cfg  Config
	tmpl *template.Template
	feed *feedCache
}

// New builds the Hold Court HTTP handler: the three-pane index page, its
// JSON APIs, and static assets. It starts a background fsnotify+interval
// watch on cfg.FeedDir (DESIGN.md: "re-scans on fsnotify + interval") that
// runs for the life of the process.
func New(cfg Config) (http.Handler, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("server: config: store is required")
	}
	if cfg.User == "" {
		cfg.User = "operator"
	}

	tmpl, err := template.ParseFS(templatesFS, "templates/*.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("server: parse templates: %w", err)
	}

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("server: static assets: %w", err)
	}

	cache := newFeedCache(cfg.FeedDir, feedRescanInterval)
	go cache.watch()

	s := &server{cfg: cfg, tmpl: tmpl, feed: cache}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("POST /api/holds/{id}/read", s.handleSetRead)
	mux.HandleFunc("POST /api/rulings", s.handleSaveRulings)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSub)))
	return mux, nil
}

// holdJSON is both the per-hold view model for the index template and the
// wire shape embedded in the page's #holds-data JSON island, so the server-
// rendered fallback and the client app agree on one set of fields.
type holdJSON struct {
	ID         string        `json:"id"`
	Title      string        `json:"title"`
	Question   string        `json:"question"`
	ReviewHTML template.HTML `json:"review_html"`
	Class      string        `json:"class"`
	Repo       string        `json:"repo"`
	PR         int           `json:"pr"`
	URL        string        `json:"url"`
	Verdict    string        `json:"verdict"`
	HeldAt     string        `json:"held_at"`
	State      string        `json:"state"` // inbox | ruled | executed | stood-down
	Unread     bool          `json:"unread"`
}

// folderJSON is one entry in the folders pane: either a selectable folder
// (state or class) or, when Heading is true, a non-selectable divider.
type folderJSON struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Count   int    `json:"count"`
	Heading bool   `json:"heading,omitempty"`
}

type pageData struct {
	Folders        []folderJSON
	SelectedFolder string
	ListHolds      []holdJSON
	SelectedHold   *holdJSON
	HoldsJSON      template.JS
	FoldersJSON    template.JS
	Keybindings    []KeyBinding
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	holds, err := s.feed.snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sort.Slice(holds, func(i, j int) bool { return holds[i].HeldAt.After(holds[j].HeldAt) })

	views := make([]holdJSON, 0, len(holds))
	for _, h := range holds {
		v, err := s.buildHoldView(h)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		views = append(views, v)
	}

	folders := buildFolders(views)

	selectedFolder := r.URL.Query().Get("folder")
	if selectedFolder == "" {
		selectedFolder = "inbox"
	}
	listHolds := filterByFolder(views, selectedFolder)

	var selected *holdJSON
	if wantID := r.URL.Query().Get("hold"); wantID != "" {
		for i := range listHolds {
			if listHolds[i].ID == wantID {
				selected = &listHolds[i]
				break
			}
		}
	}
	if selected == nil && len(listHolds) > 0 {
		selected = &listHolds[0]
	}

	holdsJSON, err := json.Marshal(views)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	foldersJSON, err := json.Marshal(folders)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := pageData{
		Folders:        folders,
		SelectedFolder: selectedFolder,
		ListHolds:      listHolds,
		SelectedHold:   selected,
		HoldsJSON:      template.JS(holdsJSON),   //nolint:gosec // encoding/json escapes <,>,& by default; safe to embed in a script tag
		FoldersJSON:    template.JS(foldersJSON), //nolint:gosec // encoding/json escapes <,>,& by default; safe to embed in a script tag
		Keybindings:    Keybindings,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "index.html.tmpl", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) buildHoldView(h *feed.Hold) (holdJSON, error) {
	reviewHTML, err := renderMarkdown(h.ReviewBodyMD)
	if err != nil {
		return holdJSON{}, fmt.Errorf("server: hold %s: %w", h.ID, err)
	}

	unread, err := s.cfg.Store.IsUnread(s.cfg.User, h.ID)
	if err != nil {
		return holdJSON{}, fmt.Errorf("server: hold %s: %w", h.ID, err)
	}

	return holdJSON{
		ID:         h.ID,
		Title:      h.Title,
		Question:   h.Question,
		ReviewHTML: reviewHTML,
		Class:      h.Class,
		Repo:       h.Repo,
		PR:         h.PR,
		URL:        h.URL,
		Verdict:    h.Verdict,
		HeldAt:     h.HeldAt.Format(time.RFC3339),
		State:      holdState(s.cfg.RulingsDir, h),
		Unread:     unread,
	}, nil
}

// holdState computes DESIGN.md's inbox/ruled/executed/stood-down state for
// h. A ruling file that can't be read is treated as "not yet ruled" rather
// than failing the whole page: one bad file on disk shouldn't take down the
// dashboard for every other hold.
func holdState(rulingsDir string, h *feed.Hold) string {
	_, ruled, err := ruling.Read(rulingsDir, h.ID)
	if err != nil || !ruled {
		if h.Resolved {
			return "stood-down"
		}
		return "inbox"
	}
	if _, executed, _ := ruling.ReadResult(rulingsDir, h.ID); executed {
		return "executed"
	}
	return "ruled"
}

// buildFolders computes the folders pane: the four fixed state folders
// (always present, even at zero count), then, if any hold carries a class,
// a divider followed by one folder per class DESIGN.md's example mockup
// shows both kinds side by side, so v1 treats them as two independent
// selectable groupings rather than picking one.
func buildFolders(views []holdJSON) []folderJSON {
	stateCounts := map[string]int{}
	classCounts := map[string]int{}
	for _, v := range views {
		stateCounts[v.State]++
		if v.Class != "" {
			classCounts[v.Class]++
		}
	}

	folders := []folderJSON{
		{ID: "inbox", Label: "Inbox", Count: stateCounts["inbox"]},
		{ID: "ruled", Label: "Ruled", Count: stateCounts["ruled"]},
		{ID: "executed", Label: "Executed", Count: stateCounts["executed"]},
		{ID: "stood-down", Label: "Stood-down", Count: stateCounts["stood-down"]},
	}

	if len(classCounts) > 0 {
		folders = append(folders, folderJSON{Label: "----", Heading: true})

		names := make([]string, 0, len(classCounts))
		for c := range classCounts {
			names = append(names, c)
		}
		sort.Strings(names)
		for _, c := range names {
			folders = append(folders, folderJSON{ID: "class:" + c, Label: c, Count: classCounts[c]})
		}
	}

	return folders
}

func filterByFolder(views []holdJSON, folderID string) []holdJSON {
	className, isClass := strings.CutPrefix(folderID, "class:")

	var out []holdJSON
	for _, v := range views {
		if isClass {
			if v.Class == className {
				out = append(out, v)
			}
			continue
		}
		if v.State == folderID {
			out = append(out, v)
		}
	}
	return out
}

// sanitizeForLog strips CR/LF from s before it reaches a log line. id and
// hold-id values here come from client-controlled request data (a URL path
// segment or JSON body field) and are not constrained to a newline-free
// charset by validation, so logging them unsanitized would let a crafted
// request forge fake-looking log lines (CWE-117).
func sanitizeForLog(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// requireJSONContentType rejects a request whose Content-Type is missing or
// is not application/json (optional parameters, e.g. "; charset=utf-8", are
// allowed) with 415, before the body is decoded.
func requireJSONContentType(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "server: Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func (s *server) handleSetRead(w http.ResponseWriter, r *http.Request) {
	if !requireJSONContentType(w, r) {
		return
	}

	id := r.PathValue("id")

	var body struct {
		Unread bool `json:"unread"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "server: decode request: "+err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	if body.Unread {
		err = s.cfg.Store.MarkUnread(s.cfg.User, id)
	} else {
		err = s.cfg.Store.MarkRead(s.cfg.User, id, time.Now())
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("hold-court: read state set for hold %s: unread=%v", sanitizeForLog(id), body.Unread) //nolint:gosec // sanitizeForLog strips CR/LF above; gosec's taint tracker doesn't see through it
	w.WriteHeader(http.StatusNoContent)
}

type rulingRequest struct {
	HoldID string `json:"hold_id"`
	Action string `json:"action"`
	Note   string `json:"note"`
}

type rulingResponse struct {
	HoldID string `json:"hold_id"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

func (s *server) handleSaveRulings(w http.ResponseWriter, r *http.Request) {
	if !requireJSONContentType(w, r) {
		return
	}

	var reqs []rulingRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		http.Error(w, "server: decode request: "+err.Error(), http.StatusBadRequest)
		return
	}

	results := make([]rulingResponse, 0, len(reqs))
	for _, item := range reqs {
		rl := ruling.Ruling{
			HoldID:  item.HoldID,
			Action:  ruling.Action(item.Action),
			Note:    item.Note,
			RuledBy: s.cfg.User,
			RuledAt: time.Now(),
		}

		res := rulingResponse{HoldID: item.HoldID}
		if err := ruling.Write(s.cfg.RulingsDir, rl); err != nil {
			res.Error = err.Error()
		} else if err := ruling.RunHook(s.cfg.OnRuling, rl); err != nil {
			res.Error = err.Error()
		} else {
			res.OK = true
			log.Printf("hold-court: ruling written for hold %s: action=%s", sanitizeForLog(rl.HoldID), rl.Action)
		}
		results = append(results, res)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}
