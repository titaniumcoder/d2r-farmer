package d2r

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed assets/templates/*.html assets/static/*.css
var webAssets embed.FS

type webCharacter struct {
	Slug  string
	Name  string
	Class string
}

type webRuneNeed struct {
	Name         string
	Count        int
	Owned        int
	Complete     bool
	CountessText string
}

type webSection struct {
	Label string
	Items []webGearItem
}

type webGearItem struct {
	Key               string
	Status            gearStatus
	RunewordBuildable bool
	RunewordMissing   bool
}

type webPageData struct {
	Characters []webCharacter
	Error      string
	ActiveSlug string
	Detail     *webDetailData
}

type webDetailData struct {
	CharacterSlug string
	CharacterName string
	ClassName     string
	Mandatory     []string
	Sections      []webSection
	Runes         []webRuneNeed
	Error         string
	Notice        string
	ImportRunning bool
	ImportCurrent int
	ImportTotal   int
	ImportItem    string
	ImportAdded   int
	ImportSkipped int
	ImportURL     string
	ImportCancel  bool
}

type importJob struct {
	mu      sync.RWMutex
	running bool
	done    bool
	url     string
	cancel  bool
	current int
	total   int
	item    string
	added   int
	skipped int
	errMsg  string
	notice  string
}

type importSnapshot struct {
	Running bool
	Done    bool
	URL     string
	Cancel  bool
	Current int
	Total   int
	Item    string
	Added   int
	Skipped int
	ErrMsg  string
	Notice  string
}

var (
	importJobsMu sync.Mutex
	importJobs   = map[string]*importJob{}
	importURLs   = map[string]string{}
)

func RunWeb(addr string) error {
	listenAddr := strings.TrimSpace(addr)
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	tmpl, err := template.New("web").Funcs(template.FuncMap{
		"join": strings.Join,
	}).ParseFS(webAssets, "assets/templates/*.html")
	if err != nil {
		return fmt.Errorf("parse web templates: %w", err)
	}

	staticFS, err := fs.Sub(webAssets, "assets/static")
	if err != nil {
		return fmt.Errorf("load static files: %w", err)
	}

	s := &webServer{templates: tmpl}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/welcome", s.handleWelcome)
	mux.HandleFunc("/characters", s.handleCreateCharacter)
	mux.HandleFunc("/characters/", s.handleCharacterRoutes)

	log.Printf("web app listening on http://localhost%s", listenAddr)
	return http.ListenAndServe(listenAddr, mux)
}

type webServer struct {
	templates *template.Template
}

func startImportJob(character string, url string) error {
	importJobsMu.Lock()
	job, ok := importJobs[character]
	if ok {
		job.mu.RLock()
		running := job.running
		job.mu.RUnlock()
		if running {
			importJobsMu.Unlock()
			return fmt.Errorf("import already running")
		}
	}

	job = &importJob{running: true, url: url}
	importJobs[character] = job
	importURLs[character] = url
	importJobsMu.Unlock()

	go func() {
		added, skipped, err := importGuideForCharacter(character, url, func(format string, args ...any) {
			log.Printf("import[%s]: "+format, append([]any{character}, args...)...)
		}, func(update importProgressUpdate) {
			job.mu.Lock()
			job.current = update.Current
			job.total = update.Total
			job.item = update.Item
			job.added = update.Imported
			job.skipped = update.Skipped
			job.mu.Unlock()
		}, func() bool {
			job.mu.RLock()
			cancel := job.cancel
			job.mu.RUnlock()
			return cancel
		})

		job.mu.Lock()
		job.running = false
		job.done = true
		job.added = added
		job.skipped = skipped
		if errors.Is(err, errImportCancelled) {
			job.errMsg = ""
			job.notice = fmt.Sprintf("import cancelled (added %d, skipped %d)", added, skipped)
		} else if err != nil {
			job.errMsg = err.Error()
			job.notice = ""
			log.Printf("import[%s]: failed: %v", character, err)
		} else {
			if added == 0 {
				job.notice = fmt.Sprintf("no new items imported (skipped %d)", skipped)
			} else {
				job.notice = fmt.Sprintf("imported %d items (skipped %d)", added, skipped)
			}
		}
		job.mu.Unlock()
	}()

	return nil
}

func getImportSnapshot(character string) (importSnapshot, bool) {
	importJobsMu.Lock()
	job, ok := importJobs[character]
	importJobsMu.Unlock()
	if !ok {
		return importSnapshot{}, false
	}

	job.mu.RLock()
	snap := importSnapshot{
		Running: job.running,
		Done:    job.done,
		URL:     job.url,
		Cancel:  job.cancel,
		Current: job.current,
		Total:   job.total,
		Item:    job.item,
		Added:   job.added,
		Skipped: job.skipped,
		ErrMsg:  job.errMsg,
		Notice:  job.notice,
	}
	job.mu.RUnlock()

	return snap, true
}

func clearImportJob(character string) {
	importJobsMu.Lock()
	delete(importJobs, character)
	importJobsMu.Unlock()
}

func cancelImportJob(character string) bool {
	importJobsMu.Lock()
	job, ok := importJobs[character]
	importJobsMu.Unlock()
	if !ok {
		return false
	}

	job.mu.Lock()
	if !job.running {
		job.mu.Unlock()
		return false
	}
	job.cancel = true
	job.item = "cancelling..."
	job.mu.Unlock()
	return true
}

func (s *webServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.Trim(r.URL.Path, "/")
	if strings.Contains(path, "/") {
		http.NotFound(w, r)
		return
	}

	chars, err := loadCharacters()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := webPageData{Characters: chars}
	if path != "" {
		detail, err := buildCharacterDetailData(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if snap, ok := getImportSnapshot(path); ok {
			detail.ImportRunning = snap.Running
			detail.ImportCancel = snap.Cancel
			detail.ImportURL = snap.URL
			detail.ImportCurrent = snap.Current
			detail.ImportTotal = snap.Total
			detail.ImportItem = snap.Item
			detail.ImportAdded = snap.Added
			detail.ImportSkipped = snap.Skipped
			if detail.Error == "" {
				detail.Error = snap.ErrMsg
			}
			if detail.Notice == "" {
				detail.Notice = snap.Notice
			}
		} else {
			importJobsMu.Lock()
			detail.ImportURL = importURLs[path]
			importJobsMu.Unlock()
		}

		data.ActiveSlug = path
		data.Detail = &detail
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.ExecuteTemplate(w, "index", data)
}

func (s *webServer) handleWelcome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.ExecuteTemplate(w, "welcome", nil)
}

func (s *webServer) handleCreateCharacter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	className := strings.TrimSpace(r.FormValue("class"))
	err := createCharacterFromWeb(name, className)

	chars, loadErr := loadCharacters()
	if loadErr != nil {
		http.Error(w, loadErr.Error(), http.StatusInternalServerError)
		return
	}

	data := webPageData{Characters: chars}
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		data.Error = err.Error()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.ExecuteTemplate(w, "character-list", data)
}

func (s *webServer) handleCharacterRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/characters/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	slug := parts[0]

	if len(parts) == 1 && r.Method == http.MethodGet {
		s.renderCharacterDetail(w, slug, "", "")
		return
	}

	if len(parts) == 2 && parts[1] == "gear" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		item := strings.TrimSpace(r.FormValue("item"))
		slot := strings.TrimSpace(r.FormValue("slot"))
		err := addGearFromWeb(slug, item, slot)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, err.Error(), "")
			return
		}

		s.renderCharacterDetail(w, slug, "", "")
		return
	}

	if len(parts) == 2 && parts[1] == "import" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		url := strings.TrimSpace(r.FormValue("url"))
		if url == "" {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, "url cannot be empty", "")
			return
		}

		if err := startImportJob(slug, url); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, err.Error(), "")
			return
		}

		s.renderCharacterDetail(w, slug, "", "import started")
		return
	}

	if len(parts) == 3 && parts[1] == "import" && parts[2] == "live" && r.Method == http.MethodGet {
		snap, ok := getImportSnapshot(slug)
		notice := ""
		errMsg := ""
		if ok {
			if snap.Done {
				notice = snap.Notice
				errMsg = snap.ErrMsg
				clearImportJob(slug)
			}
		}
		s.renderCharacterDetail(w, slug, errMsg, notice)
		return
	}

	if len(parts) == 3 && parts[1] == "import" && parts[2] == "cancel" && r.Method == http.MethodPost {
		if !cancelImportJob(slug) {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, "no running import to cancel", "")
			return
		}
		s.renderCharacterDetail(w, slug, "", "cancelling import...")
		return
	}

	if len(parts) == 2 && parts[1] == "toggle-known" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		key := strings.TrimSpace(r.FormValue("key"))
		if err := toggleKnownFromWeb(slug, key); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, err.Error(), "")
			return
		}
		s.renderCharacterDetail(w, slug, "", "")
		return
	}

	if len(parts) == 2 && parts[1] == "move-hand" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		key := strings.TrimSpace(r.FormValue("key"))
		if err := toggleHandFromWeb(slug, key); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, err.Error(), "")
			return
		}
		s.renderCharacterDetail(w, slug, "", "")
		return
	}

	if len(parts) == 2 && parts[1] == "remove" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		key := strings.TrimSpace(r.FormValue("key"))
		if err := removeGearFromWeb(slug, key); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, err.Error(), "")
			return
		}
		s.renderCharacterDetail(w, slug, "", "")
		return
	}

	if len(parts) == 2 && parts[1] == "reorder" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		sourceKey := strings.TrimSpace(r.FormValue("source_key"))
		targetKey := strings.TrimSpace(r.FormValue("target_key"))
		if err := reorderGearFromWeb(slug, sourceKey, targetKey); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, err.Error(), "")
			return
		}
		s.renderCharacterDetail(w, slug, "", "")
		return
	}

	if len(parts) == 2 && parts[1] == "runes" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		runeName := canonicalRuneName(r.FormValue("rune"))
		deltaText := strings.TrimSpace(r.FormValue("delta"))
		delta, err := strconv.Atoi(deltaText)
		if err != nil || (delta != 1 && delta != -1) {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, "invalid rune counter delta", "")
			return
		}

		if err := adjustRuneOwnedFromWeb(slug, runeName, delta); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, err.Error(), "")
			return
		}

		s.renderCharacterDetail(w, slug, "", "")
		return
	}

	if len(parts) == 3 && parts[1] == "base" && parts[2] == "remove" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}

		key := strings.TrimSpace(r.FormValue("key"))
		baseName := strings.TrimSpace(r.FormValue("base"))
		if err := removeBaseFromWeb(slug, key, baseName); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, err.Error(), "")
			return
		}

		s.renderCharacterDetail(w, slug, "", "")
		return
	}

	http.NotFound(w, r)
}

func (s *webServer) renderCharacterDetail(w http.ResponseWriter, slug string, errMsg string, notice string) {
	detail, err := buildCharacterDetailData(slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	detail.Error = errMsg
	detail.Notice = notice
	if snap, ok := getImportSnapshot(slug); ok {
		detail.ImportRunning = snap.Running
		detail.ImportCancel = snap.Cancel
		detail.ImportURL = snap.URL
		detail.ImportCurrent = snap.Current
		detail.ImportTotal = snap.Total
		detail.ImportItem = snap.Item
		detail.ImportAdded = snap.Added
		detail.ImportSkipped = snap.Skipped
		if detail.Error == "" {
			detail.Error = snap.ErrMsg
		}
		if detail.Notice == "" {
			detail.Notice = snap.Notice
		}
	} else {
		importJobsMu.Lock()
		detail.ImportURL = importURLs[slug]
		importJobsMu.Unlock()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.ExecuteTemplate(w, "character-detail", detail)
}

func loadCharacters() ([]webCharacter, error) {
	charsDir := filepath.Join("data", "chars")
	entries, err := os.ReadDir(charsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read chars directory: %w", err)
	}

	out := make([]webCharacter, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		slug := strings.TrimSuffix(entry.Name(), ".yaml")
		if slug == "" {
			continue
		}

		data, err := readCharacterData(slug)
		if err != nil {
			continue
		}

		name := stringValue(data["name"])
		if name == "" {
			name = slug
		}

		out = append(out, webCharacter{
			Slug:  slug,
			Name:  name,
			Class: stringValue(data["class"]),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(out[i].Name)
		right := strings.ToLower(out[j].Name)
		if left == right {
			return out[i].Slug < out[j].Slug
		}
		return left < right
	})

	return out, nil
}

func createCharacterFromWeb(name string, className string) error {
	if name == "" {
		return fmt.Errorf("character name cannot be empty")
	}
	if className == "" {
		return fmt.Errorf("class cannot be empty")
	}

	slug := slugifyName(name)
	if slug == "" {
		return fmt.Errorf("character name %q has no valid filename characters", name)
	}

	charsDir := filepath.Join("data", "chars")
	if err := os.MkdirAll(charsDir, 0o755); err != nil {
		return fmt.Errorf("create chars directory: %w", err)
	}

	characterPath := filepath.Join(charsDir, slug+".yaml")
	if _, err := os.Stat(characterPath); err == nil {
		return fmt.Errorf("character already exists: %s", characterPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check character file: %w", err)
	}

	content, err := buildCharacterYAML(name, className, nil)
	if err != nil {
		return err
	}

	if err := os.WriteFile(characterPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write character file: %w", err)
	}
	return nil
}

func addGearFromWeb(character string, gear string, selectedSlot string) error {
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}
	if gear == "" {
		return fmt.Errorf("gear cannot be empty")
	}

	slotHint, err := normalizeSelectedSlotHint(selectedSlot)
	if err != nil {
		return err
	}

	llmSlotHint := slotHint
	if slotHint == "weapon_swap" || slotHint == "weapon_swap_offhand" {
		llmSlotHint = "weapon"
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	charClass := stringValue(data["class"])
	if charClass == "" {
		return fmt.Errorf("character %q has no class set", character)
	}

	cfg, err := readConfig()
	if err != nil {
		return err
	}

	entry, err := resolveGearWithLLM(gear, charClass, llmSlotHint, cfg)
	if err != nil {
		return fmt.Errorf("resolve gear details: %w", err)
	}

	if strings.TrimSpace(fmt.Sprint(entry["exact_name"])) == "" {
		entry["exact_name"] = gear
	}
	if strings.TrimSpace(fmt.Sprint(entry["query"])) == "" {
		entry["query"] = gear
	}

	normalizeResolvedSlotAndRole(entry)
	entry["weapon_swap"] = false
	if slotHint != "" {
		switch slotHint {
		case "weapon_swap":
			entry["slot"] = "weapon"
			entry["weapon_swap"] = true
			entry["swap_role"] = "main"
		case "weapon_swap_offhand":
			entry["slot"] = "weapon"
			entry["weapon_swap"] = true
			entry["swap_role"] = "offhand"
		case "offhand":
			entry["slot"] = "weapon"
			entry["swap_role"] = "offhand"
		case "weapon":
			entry["slot"] = "weapon"
			entry["swap_role"] = "main"
		default:
			entry["slot"] = normalizeSlotName(slotHint)
		}
	}

	gearList := coerceGearEntries(data["gear"])
	gearList = append(gearList, entry)
	setGearEntries(data, gearList)

	if err := writeCharacterData(character, data); err != nil {
		return err
	}

	return nil
}

func normalizeSelectedSlotHint(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "automatic", "auto":
		return "", nil
	case "weapon", "offhand", "weapon_swap", "weapon_swap_offhand", "head", "armor", "belt", "ring", "amulet", "inventory":
		return value, nil
	default:
		return "", fmt.Errorf("invalid slot selection")
	}
}

func parseGearKey(key string, length int) (int, error) {
	idx, err := strconv.Atoi(strings.TrimSpace(key))
	if err != nil {
		return -1, fmt.Errorf("invalid item key")
	}
	if idx < 0 || idx >= length {
		return -1, fmt.Errorf("item key out of range")
	}
	return idx, nil
}

func toggleKnownFromWeb(character string, key string) error {
	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	idx, err := parseGearKey(key, len(gearList))
	if err != nil {
		return err
	}

	if gearFound(gearList[idx]) {
		gearList[idx]["found"] = false
		delete(gearList[idx], "found_at")
	} else {
		gearList[idx]["found"] = true
	}

	setGearEntries(data, gearList)
	return writeCharacterData(character, data)
}

func toggleHandFromWeb(character string, key string) error {
	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	idx, err := parseGearKey(key, len(gearList))
	if err != nil {
		return err
	}

	if slotForEntry(gearList[idx]) != "weapon" {
		return fmt.Errorf("hand can only be changed for weapon items")
	}

	if normalizeSwapRole(stringValue(gearList[idx]["swap_role"])) == "offhand" {
		gearList[idx]["swap_role"] = "main"
	} else {
		gearList[idx]["swap_role"] = "offhand"
	}

	setGearEntries(data, gearList)
	return writeCharacterData(character, data)
}

func removeGearFromWeb(character string, key string) error {
	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	idx, err := parseGearKey(key, len(gearList))
	if err != nil {
		return err
	}

	gearList = append(gearList[:idx], gearList[idx+1:]...)
	setGearEntries(data, gearList)
	return writeCharacterData(character, data)
}

func reorderGearFromWeb(character string, sourceKey string, targetKey string) error {
	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	if len(gearList) < 2 {
		return nil
	}

	source, err := parseGearKey(sourceKey, len(gearList))
	if err != nil {
		return err
	}
	target, err := parseGearKey(targetKey, len(gearList))
	if err != nil {
		return err
	}
	if source == target {
		return nil
	}

	item := gearList[source]
	gearList = append(gearList[:source], gearList[source+1:]...)
	if source < target {
		target--
	}
	if target < 0 {
		target = 0
	}
	if target > len(gearList) {
		target = len(gearList)
	}

	front := append([]map[string]any{}, gearList[:target]...)
	front = append(front, item)
	gearList = append(front, gearList[target:]...)

	setGearEntries(data, gearList)
	return writeCharacterData(character, data)
}

func adjustRuneOwnedFromWeb(character string, runeName string, delta int) error {
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}
	if runeName == "" {
		return fmt.Errorf("rune cannot be empty")
	}
	if delta != 1 && delta != -1 {
		return fmt.Errorf("delta must be +1 or -1")
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	owned := readRuneOwnedCounts(data)
	next := owned[runeName] + delta
	if next < 0 {
		next = 0
	}
	if next == 0 {
		delete(owned, runeName)
	} else {
		owned[runeName] = next
	}
	writeRuneOwnedCounts(data, owned)

	return writeCharacterData(character, data)
}

func removeBaseFromWeb(character string, key string, baseName string) error {
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}
	if baseName == "" {
		return fmt.Errorf("base cannot be empty")
	}

	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	idx, err := parseGearKey(key, len(gearList))
	if err != nil {
		return err
	}

	match := normalizeGearLookup(baseName)
	if match == "" {
		return fmt.Errorf("base cannot be empty")
	}

	bases := stringSliceValue(gearList[idx]["possible_bases"])
	filteredBases := make([]string, 0, len(bases))
	for _, candidate := range bases {
		if normalizeGearLookup(candidate) == match {
			continue
		}
		filteredBases = append(filteredBases, candidate)
	}
	gearList[idx]["possible_bases"] = filteredBases

	rawDetails, hasDetails := gearList[idx]["possible_bases_details"]
	if hasDetails {
		switch typed := rawDetails.(type) {
		case []any:
			filtered := make([]any, 0, len(typed))
			for _, item := range typed {
				entry, ok := normalizeEntryMap(item)
				if !ok {
					continue
				}
				if normalizeGearLookup(stringValue(entry["name"])) == match {
					continue
				}
				filtered = append(filtered, entry)
			}
			gearList[idx]["possible_bases_details"] = filtered
		}
	}

	setGearEntries(data, gearList)
	return writeCharacterData(character, data)
}

func readRuneOwnedCounts(data map[string]any) map[string]int {
	owned := map[string]int{}
	raw, ok := data["runes_owned"]
	if !ok || raw == nil {
		return owned
	}

	entries, ok := raw.(map[string]any)
	if !ok {
		return owned
	}

	for key, value := range entries {
		runeName := canonicalRuneName(key)
		if runeName == "" {
			continue
		}

		count := 0
		switch typed := value.(type) {
		case int:
			count = typed
		case int64:
			count = int(typed)
		case float64:
			count = int(typed)
		default:
			parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
			if err != nil {
				continue
			}
			count = parsed
		}

		if count > 0 {
			owned[runeName] = count
		}
	}

	return owned
}

func writeRuneOwnedCounts(data map[string]any, owned map[string]int) {
	if len(owned) == 0 {
		delete(data, "runes_owned")
		return
	}

	out := map[string]any{}
	for runeName, count := range owned {
		if count > 0 {
			out[runeName] = count
		}
	}
	if len(out) == 0 {
		delete(data, "runes_owned")
		return
	}

	data["runes_owned"] = out
}

func buildCharacterDetailData(slug string) (webDetailData, error) {
	data, err := readCharacterData(slug)
	if err != nil {
		return webDetailData{}, err
	}

	name := stringValue(data["name"])
	if name == "" {
		name = slug
	}

	sections := make([]webSection, 0)
	gearList := coerceGearEntries(data["gear"])
	runeOwned := readRuneOwnedCounts(data)
	statuses := buildGearStatuses(gearList)
	allItems := make([]webGearItem, 0, len(statuses))
	for idx, status := range statuses {
		buildable, missing := runewordCraftStatus(status, runeOwned)
		allItems = append(allItems, webGearItem{
			Key:               strconv.Itoa(idx),
			Status:            status,
			RunewordBuildable: buildable,
			RunewordMissing:   missing,
		})
	}

	if len(allItems) == 0 {
		for _, label := range []string{"weapon", "offhand", "weapon swap", "weapon swap offhand", "head", "armor", "belt", "ring", "amulet", "inventory"} {
			sections = append(sections, webSection{Label: label})
		}
	} else {
		bySlot := map[string][]webGearItem{}
		for _, item := range allItems {
			bySlot[item.Status.Slot] = append(bySlot[item.Status.Slot], item)
		}

		weapon := bySlot["weapon"]
		sections = append(sections, webSection{Label: "weapon", Items: filterWebItems(weapon, func(s gearStatus) bool { return !s.IsSwap && s.SwapRole != "offhand" })})
		sections = append(sections, webSection{Label: "offhand", Items: filterWebItems(weapon, func(s gearStatus) bool { return !s.IsSwap && s.SwapRole == "offhand" })})
		sections = append(sections, webSection{Label: "weapon swap", Items: filterWebItems(weapon, func(s gearStatus) bool { return s.IsSwap && s.SwapRole != "offhand" })})
		sections = append(sections, webSection{Label: "weapon swap offhand", Items: filterWebItems(weapon, func(s gearStatus) bool { return s.IsSwap && s.SwapRole == "offhand" })})

		for _, slot := range []string{"head", "armor", "belt", "ring", "amulet", "inventory"} {
			sections = append(sections, webSection{Label: slot, Items: bySlot[slot]})
		}
	}

	mandatory := readMandatoryRequirements(data)

	return webDetailData{
		CharacterSlug: slug,
		CharacterName: name,
		ClassName:     stringValue(data["class"]),
		Mandatory:     mandatory,
		Sections:      sections,
		Runes:         buildRuneNeeds(data),
	}, nil
}

func filterWebItems(items []webGearItem, keep func(gearStatus) bool) []webGearItem {
	out := make([]webGearItem, 0)
	for _, item := range items {
		if keep(item.Status) {
			out = append(out, item)
		}
	}
	return out
}

func runewordCraftStatus(status gearStatus, owned map[string]int) (bool, bool) {
	if strings.ToLower(status.Kind) != "runeword" {
		return false, false
	}

	need := map[string]int{}
	for _, runeName := range status.Runes {
		canonical := canonicalRuneName(runeName)
		if canonical == "" {
			continue
		}
		need[canonical]++
	}

	if len(need) == 0 {
		return false, true
	}

	for runeName, count := range need {
		if owned[runeName] < count {
			return false, true
		}
	}

	return true, false
}

func buildRuneNeeds(data map[string]any) []webRuneNeed {
	gearList := coerceGearEntries(data["gear"])
	counts := map[string]int{}
	owned := readRuneOwnedCounts(data)
	order := make([]string, 0)

	for _, entry := range gearList {
		if gearFound(entry) {
			continue
		}

		for _, runeName := range stringSliceValue(entry["runes"]) {
			runeName = canonicalRuneName(runeName)
			if runeName == "" {
				continue
			}
			if _, ok := counts[runeName]; !ok {
				order = append(order, runeName)
			}
			counts[runeName]++
		}
	}

	sort.Slice(order, func(i, j int) bool {
		left := runeDifficultyOrder(order[i])
		right := runeDifficultyOrder(order[j])
		if left == right {
			return order[i] < order[j]
		}
		return left < right
	})

	out := make([]webRuneNeed, 0, len(order))
	for _, runeName := range order {
		count := counts[runeName]
		countess := countessDifficultiesForRune(runeName)
		minDifficulty := ""
		if len(countess) > 0 {
			minDifficulty = countess[0]
		}
		ownedCount := owned[runeName]
		out = append(out, webRuneNeed{
			Name:         runeName,
			Count:        count,
			Owned:        ownedCount,
			Complete:     ownedCount >= count,
			CountessText: minDifficulty,
		})
	}

	return out
}
