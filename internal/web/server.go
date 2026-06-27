// Package web wires the HTTP routes and renders the html/template UI. It is a
// thin layer: handlers call the read-only data layer (the Backups interface)
// and render templates. It owns no kopia logic, so it can be tested against a
// fake Backups implementation.
package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/nicojeske/kopia-browser/internal/config"
	"github.com/nicojeske/kopia-browser/internal/kopia"
)

// Backups is the read-only data layer the handlers depend on. *kopia.Manager
// implements it in production; tests supply an in-memory fake.
type Backups interface {
	ListNamespaces(ctx context.Context) ([]string, error)
	ListSnapshots(ctx context.Context, ns string) ([]kopia.SnapshotInfo, error)
	Dir(ctx context.Context, ns, snapID, path string) ([]kopia.DirEntry, error)
	// OpenFile returns a seekable stream for a single file within a snapshot
	// plus its metadata. Caller must Close the reader. Returns kopia.ErrNotAFile
	// when the path resolves to a directory (including empty path = root).
	OpenFile(ctx context.Context, ns, snapID, path string) (io.ReadSeekCloser, kopia.DirEntry, error)
	// TarDir streams a plain tar archive of the directory subtree rooted at path
	// (empty = snapshot root) into w.
	TarDir(ctx context.Context, ns, snapID, path string, w io.Writer) error
}

// Stats is the dashboard statistics cache. *kopia.StatsCache implements it;
// tests supply an in-memory fake.
type Stats interface {
	Get() kopia.StatsSnapshot
}

// VolumeInfo summarises one Velero PVC volume within a namespace.
// Name is the raw volume tag value ("" for snapshots that carry no volume tag).
// Display is the human-readable label: "(no volume)" when Name is empty.
type VolumeInfo struct {
	Name    string    // raw Tags["volume"] value; "" for untagged snapshots
	Display string    // shown in UI; "(no volume)" when Name == ""
	Count   int       // total snapshots for this volume
	Latest  time.Time // start time of the newest snapshot
}

// crumb is one level of the directory breadcrumb.
type crumb struct {
	Name string
	Href string // empty = current location (rendered as plain text, not a link)
}

// statCard is one of the four top-of-dashboard summary tiles.
type statCard struct {
	Label string
	Value string
	Unit  string
}

// nsCard is a namespace card shown on the dashboard grid.
type nsCard struct {
	Name      string
	Volumes   int
	Snapshots int
	SizeLabel string // pre-formatted (humanBytes or "—")
	BarPct    int    // 4–100; width % of the largest namespace's size
	LastLabel string // pre-formatted relative time or "—"
	SizeBytes int64  // raw value for data-* attr (client-side sort)
	LastUnix  int64  // Unix timestamp for data-* attr (client-side sort)
}

// sidebarNSItem is one namespace nav row in the sidebar.
type sidebarNSItem struct {
	Name      string
	Snapshots int  // 0 when stats not yet ready
	Active    bool // true for the namespace the user is currently viewing
}

// sidebarRepoSegment is one coloured strip in the sidebar composition bar.
type sidebarRepoSegment struct {
	Pct int // width percentage of the bar (2–100)
}

// sidebarRepo is the "Repository" footer block in the sidebar.
type sidebarRepo struct {
	Size     string               // humanBytes of total stored size, or "—"
	NSCount  int                  // total namespace count
	Snapshots string              // humanCount of total snapshots
	Segments []sidebarRepoSegment // up to 5 top-ns segments for the composition bar
	Ready    bool                 // false = stats not yet available
}

// server holds parsed templates, config, the data layer, and the stats cache.
type server struct {
	cfg     *config.Config
	backups Backups
	stats   Stats
	tpl     *template.Template
}

// NewServer builds the HTTP handler. templates holds the *.html files and
// static holds the embedded JS/CSS (see package assets). Templates are parsed
// once at startup.
func NewServer(cfg *config.Config, backups Backups, stats Stats, templates, static fs.FS) (http.Handler, error) {
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(templates, "*.html")
	if err != nil {
		return nil, err
	}
	s := &server{cfg: cfg, backups: backups, stats: stats, tpl: tpl}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /repo/{ns}", s.handleVolumes)
	mux.HandleFunc("GET /repo/{ns}/vol/{volume...}", s.handleSnapshots)
	mux.HandleFunc("GET /repo/{ns}/snap/{id}/browse/{path...}", s.handleBrowse)
	mux.HandleFunc("GET /repo/{ns}/snap/{id}/download/{path...}", s.handleDownload)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	return mux, nil
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

// handleIndex lists the namespaces (one kopia repo each), with enriched stats
// from the background cache (stat cards, per-ns mini-stats, size bars).
func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	nsList, err := s.backups.ListNamespaces(r.Context())
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "listing namespaces", err)
		return
	}

	snap := s.stats.Get()

	// Build a name → NamespaceStats lookup from the cache.
	statsByNS := make(map[string]kopia.NamespaceStats, len(snap.Namespaces))
	for _, ns := range snap.Namespaces {
		statsByNS[ns.Name] = ns
	}

	// Newest last-backup across all namespaces (for the "Last backup" stat card).
	var newestBackup time.Time
	for _, ns := range snap.Namespaces {
		if ns.LastBackup.After(newestBackup) {
			newestBackup = ns.LastBackup
		}
	}

	// Build the four summary stat cards.
	cards4 := []statCard{
		{Label: "Namespaces", Value: humanCount(len(nsList)), Unit: ""},
		{Label: "Total snapshots", Value: func() string {
			if snap.Ready {
				return humanCount(snap.TotalSnapshots)
			}
			return "—"
		}(), Unit: ""},
		{Label: "Stored", Value: func() string {
			if snap.Ready {
				return humanBytes(snap.TotalSize)
			}
			return "—"
		}(), Unit: ""},
		{Label: "Last backup", Value: func() string {
			if snap.Ready && !newestBackup.IsZero() {
				return humanRel(newestBackup)
			}
			return "—"
		}(), Unit: ""},
	}

	// Build per-namespace cards. Always iterate nsList (authoritative order);
	// augment with stats when available.
	maxSize := snap.MaxSize
	if maxSize == 0 {
		maxSize = 1 // guard against div-by-zero
	}
	nsCards := make([]nsCard, 0, len(nsList))
	for _, nsName := range nsList {
		card := nsCard{Name: nsName, BarPct: 4, SizeLabel: "—", LastLabel: "—"}
		if st, ok := statsByNS[nsName]; ok && snap.Ready {
			card.Volumes = st.Volumes
			card.Snapshots = st.Snapshots
			card.SizeLabel = humanBytes(st.SizeBytes)
			card.SizeBytes = st.SizeBytes
			card.LastLabel = humanRel(st.LastBackup)
			if !st.LastBackup.IsZero() {
				card.LastUnix = st.LastBackup.Unix()
			}
			barPct := int(st.SizeBytes * 100 / maxSize)
			if barPct < 4 {
				barPct = 4
			}
			card.BarPct = barPct
		}
		nsCards = append(nsCards, card)
	}

	data := map[string]any{
		"Title":     "Namespaces",
		"StatCards": cards4,
		"Cards":     nsCards,
		"NotReady":  !snap.Ready,
	}
	s.injectSidebarData(data, nsList, "")
	s.render(w, "namespaces.html", data)
}

// handleVolumes lists the distinct Velero volumes (PVCs) within a namespace.
func (s *server) handleVolumes(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	allSnaps, err := s.backups.ListSnapshots(r.Context(), ns)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "listing snapshots", err)
		return
	}

	// Bucket snapshots by volume name, preserving newest-first insertion order.
	type bucket struct {
		count  int
		latest time.Time
	}
	byVol := map[string]*bucket{}
	var order []string
	for _, snap := range allSnaps {
		vol := snap.Volume
		if _, seen := byVol[vol]; !seen {
			order = append(order, vol)
			byVol[vol] = &bucket{}
		}
		b := byVol[vol]
		b.count++
		if snap.StartTime.After(b.latest) {
			b.latest = snap.StartTime
		}
	}

	// Sort alphabetically; untagged ("") goes last.
	sort.Slice(order, func(i, j int) bool {
		if order[i] == "" {
			return false
		}
		if order[j] == "" {
			return true
		}
		return order[i] < order[j]
	})

	vols := make([]VolumeInfo, 0, len(order))
	for _, name := range order {
		b := byVol[name]
		display := name
		if display == "" {
			display = "(no volume)"
		}
		vols = append(vols, VolumeInfo{Name: name, Display: display, Count: b.count, Latest: b.latest})
	}

	nsList, nsErr := s.backups.ListNamespaces(r.Context())
	if nsErr != nil {
		slog.Warn("handleVolumes: listing namespaces for sidebar failed", "err", nsErr)
		nsList = nil
	}

	data := map[string]any{
		"Title":     ns,
		"Namespace": ns,
		"Volumes":   vols,
	}
	s.injectSidebarData(data, nsList, ns)
	s.render(w, "volumes.html", data)
}

// handleSnapshots lists the snapshots for a specific volume within a namespace.
func (s *server) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	volume := r.PathValue("volume")

	allSnaps, err := s.backups.ListSnapshots(r.Context(), ns)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "listing snapshots", err)
		return
	}

	var snaps []kopia.SnapshotInfo
	for _, snap := range allSnaps {
		if snap.Volume == volume {
			snaps = append(snaps, snap)
		}
	}

	display := volume
	if display == "" {
		display = "(no volume)"
	}

	nsList, nsErr := s.backups.ListNamespaces(r.Context())
	if nsErr != nil {
		slog.Warn("handleSnapshots: listing namespaces for sidebar failed", "err", nsErr)
		nsList = nil
	}

	data := map[string]any{
		"Title":     ns + " / " + display,
		"Namespace": ns,
		"Volume":    volume,
		"Display":   display,
		"Snapshots": snaps,
	}
	s.injectSidebarData(data, nsList, ns)
	s.render(w, "snapshots.html", data)
}

// handleBrowse renders the directory listing for a path inside a snapshot.
// When the request carries an HX-Request header (htmx), only the inner
// browse-content fragment is returned so htmx can swap it in-place.
func (s *server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	snapID := r.PathValue("id")
	rawPath := r.PathValue("path")

	cleanPath, segs, err := cleanBrowsePath(rawPath)
	if err != nil {
		s.renderError(w, http.StatusBadRequest, "invalid path", err)
		return
	}

	entries, err := s.backups.Dir(r.Context(), ns, snapID, cleanPath)
	if err != nil {
		s.renderError(w, http.StatusInternalServerError, "listing directory", err)
		return
	}

	browseBase := fmt.Sprintf("/repo/%s/snap/%s/browse", ns, snapID)
	if cleanPath != "" {
		browseBase += "/" + cleanPath
	}

	downloadBase := fmt.Sprintf("/repo/%s/snap/%s/download", ns, snapID)
	if cleanPath != "" {
		downloadBase += "/" + cleanPath
	}

	snapDisplay := snapID
	if len(snapDisplay) > 8 {
		snapDisplay = snapDisplay[:8]
	}
	rootHref := fmt.Sprintf("/repo/%s/snap/%s/browse/", ns, snapID)
	crumbs := []crumb{
		{Name: "Namespaces", Href: "/"},
		{Name: ns, Href: fmt.Sprintf("/repo/%s", ns)},
	}
	if len(segs) == 0 {
		crumbs = append(crumbs, crumb{Name: snapDisplay})
	} else {
		crumbs = append(crumbs, crumb{Name: snapDisplay, Href: rootHref})
		acc := ""
		for i, seg := range segs {
			if acc != "" {
				acc += "/"
			}
			acc += seg
			href := ""
			if i < len(segs)-1 {
				href = fmt.Sprintf("/repo/%s/snap/%s/browse/%s", ns, snapID, acc)
			}
			crumbs = append(crumbs, crumb{Name: seg, Href: href})
		}
	}

	title := ns + " / " + snapDisplay
	if cleanPath != "" {
		title += " / " + cleanPath
	}
	data := map[string]any{
		"Title":        title,
		"Namespace":    ns,
		"SnapID":       snapID,
		"Path":         cleanPath,
		"Crumbs":       crumbs,
		"Entries":      entries,
		"BrowseBase":   browseBase,
		"DownloadBase": downloadBase,
	}

	if r.Header.Get("HX-Request") == "true" {
		// htmx fragment: browse-content never reads sidebar data; skip the S3 call.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.tpl.ExecuteTemplate(w, "browse-content", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	nsList, nsErr := s.backups.ListNamespaces(r.Context())
	if nsErr != nil {
		slog.Warn("handleBrowse: listing namespaces for sidebar failed", "err", nsErr)
	}
	s.injectSidebarData(data, nsList, ns)
	s.render(w, "browse.html", data)
}

// handleDownload serves a snapshot entry as a download.
func (s *server) handleDownload(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("ns")
	snapID := r.PathValue("id")
	rawPath := r.PathValue("path")

	cleanPath, _, err := cleanBrowsePath(rawPath)
	if err != nil {
		s.renderError(w, http.StatusBadRequest, "invalid path", err)
		return
	}

	rc, entry, fileErr := s.backups.OpenFile(r.Context(), ns, snapID, cleanPath)
	switch {
	case fileErr == nil:
		defer rc.Close()
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, entry.Name))
		http.ServeContent(w, r, entry.Name, entry.ModTime, rc)

	case errors.Is(fileErr, kopia.ErrNotAFile):
		base := path.Base(cleanPath)
		if cleanPath == "" {
			base = ns
		}
		tarName := base + ".tar"
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, tarName))
		if terr := s.backups.TarDir(r.Context(), ns, snapID, cleanPath, w); terr != nil {
			slog.Error("handleDownload: tar streaming failed", "ns", ns, "snap", snapID, "path", cleanPath, "err", terr)
		}

	case errors.Is(fileErr, kopia.ErrNotFound):
		s.renderError(w, http.StatusNotFound, "not found", fileErr)

	default:
		s.renderError(w, http.StatusInternalServerError, "opening file", fileErr)
	}
}

// injectSidebarData populates data with SidebarNS and SidebarRepo, merging
// the authoritative namespace list with snapshot counts from the stats cache.
// nsList is the pre-fetched namespace list (may be nil on error; degrades gracefully).
func (s *server) injectSidebarData(data map[string]any, nsList []string, activeNS string) {
	snap := s.stats.Get()

	// Build snapshot-count lookup from the cache.
	snapByNS := make(map[string]int, len(snap.Namespaces))
	for _, ns := range snap.Namespaces {
		snapByNS[ns.Name] = ns.Snapshots
	}

	items := make([]sidebarNSItem, 0, len(nsList))
	for _, ns := range nsList {
		items = append(items, sidebarNSItem{
			Name:      ns,
			Snapshots: snapByNS[ns],
			Active:    ns == activeNS,
		})
	}
	data["SidebarNS"] = items

	// Build the repository footer.
	repo := sidebarRepo{
		NSCount:   snap.NamespaceCount,
		Snapshots: humanCount(snap.TotalSnapshots),
		Ready:     snap.Ready,
	}
	if snap.Ready {
		repo.Size = humanBytes(snap.TotalSize)
		// Composition bar: up to 5 top namespaces as proportional segments.
		const maxSegs = 5
		if snap.TotalSize > 0 {
			n := len(snap.Namespaces)
			if n > maxSegs {
				n = maxSegs
			}
			for i := 0; i < n; i++ {
				pct := int(snap.Namespaces[i].SizeBytes * 100 / snap.TotalSize)
				if pct < 2 {
					pct = 2
				}
				repo.Segments = append(repo.Segments, sidebarRepoSegment{Pct: pct})
			}
		}
	} else {
		repo.Size = "—"
	}
	data["SidebarRepo"] = repo
}

// cleanBrowsePath normalises a user-supplied path value from the URL.
func cleanBrowsePath(raw string) (string, []string, error) {
	if raw == "" {
		return "", nil, nil
	}
	cleaned := strings.TrimPrefix(path.Clean("/"+raw), "/")
	if cleaned == "" || cleaned == "." {
		return "", nil, nil
	}
	segs := strings.Split(cleaned, "/")
	for _, seg := range segs {
		if seg == ".." || seg == "." || seg == "" {
			return "", nil, fmt.Errorf("invalid path segment %q", seg)
		}
	}
	return cleaned, segs, nil
}

func (s *server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) renderError(w http.ResponseWriter, code int, what string, err error) {
	msg := fmt.Sprintf("%s: %v", what, err)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if terr := s.tpl.ExecuteTemplate(w, "error.html", map[string]any{
		"Title":   "Error",
		"Code":    code,
		"Message": msg,
	}); terr != nil {
		slog.Error("renderError: template execute failed", "err", terr)
	}
}

// templateFuncs are helpers available in all templates.
var templateFuncs = template.FuncMap{
	"humanBytes":    humanBytes,
	"humanTime":     func(t time.Time) string { return t.Format("2006-01-02 15:04:05") },
	"humanRel":      humanRel,
	"humanCount":    humanCount,
	"urlPathEscape": url.PathEscape,
	"fileCategory":  fileCategory,
	"sliceStr":      func(s string, n int) string { // sliceStr returns the first n runes of s
		if len(s) <= n {
			return s
		}
		return s[:n]
	},
}

// fileCategory maps a filename to a display category string used to select
// the appropriate icon and colour class in the browse template. The returned
// value is one of: image, video, audio, archive, code, pdf, doc, sheet, file.
func fileCategory(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico", ".tiff", ".tif", ".heic", ".heif", ".avif":
		return "image"
	case ".mp4", ".mkv", ".mov", ".avi", ".webm", ".wmv", ".flv", ".m4v", ".ogv", ".mts":
		return "video"
	case ".mp3", ".wav", ".flac", ".ogg", ".m4a", ".aac", ".opus", ".wma", ".aiff":
		return "audio"
	case ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".zst", ".lz4", ".lzma":
		return "archive"
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".rb", ".rs", ".java", ".c", ".cpp", ".cc", ".h", ".hpp",
		".sh", ".bash", ".zsh", ".fish", ".ps1", ".yaml", ".yml", ".json", ".toml", ".xml", ".html", ".htm",
		".css", ".scss", ".sass", ".sql", ".lua", ".php", ".swift", ".kt", ".cs", ".dart", ".vim", ".tf":
		return "code"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx", ".odt", ".rtf", ".md", ".txt", ".rst", ".tex":
		return "doc"
	case ".xls", ".xlsx", ".ods", ".csv", ".tsv":
		return "sheet"
	default:
		return "file"
	}
}

// humanBytes renders a byte count as a human-readable binary size.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// humanRel renders a time as a relative string ("2h ago", "3d ago", etc.).
// Returns "—" for zero time.
func humanRel(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw ago", int(d.Hours()/24/7))
	}
}

// humanCount renders an integer with thousands separators ("1,204").
func humanCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	result := make([]byte, 0, len(s)+len(s)/3)
	start := len(s) % 3
	if start == 0 {
		start = 3
	}
	result = append(result, s[:start]...)
	for i := start; i < len(s); i += 3 {
		result = append(result, ',')
		result = append(result, s[i:i+3]...)
	}
	return string(result)
}
