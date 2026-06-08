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

type webModelOption struct {
	ID      string
	Pricing string
}

type settingsPageData struct {
	APIKey        string
	SelectedModel string
	Models        []webModelOption
	Error         string
	Notice        string
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
	mux.HandleFunc("/settings", s.handleSettingsPage)
	mux.HandleFunc("/settings/save", s.handleSaveSettings)
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
	if path == "settings" {
		http.Redirect(w, r, "/settings", http.StatusFound)
		return
	}
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

func (s *webServer) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg, err := readConfig()
	models := fallbackModels()
	modelsErr := ""
	if err == nil {
		live, liveErr := availableModels(cfg.OpenAI.APIKey)
		if liveErr != nil {
			modelsErr = fmt.Sprintf("could not load models from OpenAI API: %v", liveErr)
		} else {
			models = live
		}
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = s.templates.ExecuteTemplate(w, "settings-page", settingsPageData{Error: err.Error(), Models: models})
		return
	}
	models = ensureSelectedModel(models, cfg.OpenAI.Model)

	data := settingsPageData{
		APIKey:        cfg.OpenAI.APIKey,
		SelectedModel: cfg.OpenAI.Model,
		Models:        models,
		Error:         modelsErr,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.ExecuteTemplate(w, "settings-page", data)
}

func (s *webServer) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	model := strings.TrimSpace(r.FormValue("model"))
	models := fallbackModels()
	if strings.TrimSpace(apiKey) != "" {
		if live, liveErr := availableModels(apiKey); liveErr == nil {
			models = live
		}
	}
	models = ensureSelectedModel(models, model)
	if apiKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = s.templates.ExecuteTemplate(w, "settings-page", settingsPageData{
			APIKey:        "",
			SelectedModel: model,
			Models:        models,
			Error:         "api key cannot be empty",
		})
		return
	}
	if model == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = s.templates.ExecuteTemplate(w, "settings-page", settingsPageData{
			APIKey:        apiKey,
			SelectedModel: "",
			Models:        models,
			Error:         "model cannot be empty",
		})
		return
	}

	current, err := readConfig()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = s.templates.ExecuteTemplate(w, "settings-page", settingsPageData{Error: err.Error(), Models: models})
		return
	}

	current.OpenAI.APIKey = apiKey
	current.OpenAI.Model = model
	if err := writeConfig(current); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = s.templates.ExecuteTemplate(w, "settings-page", settingsPageData{
			APIKey:        apiKey,
			SelectedModel: model,
			Models:        models,
			Error:         err.Error(),
		})
		return
	}

	if live, liveErr := availableModels(apiKey); liveErr == nil {
		models = ensureSelectedModel(live, model)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.ExecuteTemplate(w, "settings-page", settingsPageData{
		APIKey:        apiKey,
		SelectedModel: model,
		Models:        models,
		Notice:        "settings saved",
	})
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
	if err == nil {
		slug := slugifyName(name)
		if slug != "" {
			w.Header().Set("HX-Redirect", "/"+slug)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

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
		merc := strings.EqualFold(strings.TrimSpace(r.FormValue("merc")), "on")
		err := addGearFromWeb(slug, item, merc)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			s.renderCharacterDetail(w, slug, err.Error(), "")
			return
		}

		s.renderCharacterDetail(w, slug, "", "")
		return
	}

	if len(parts) == 2 && parts[1] == "delete" && r.Method == http.MethodPost {
		err := deleteCharacterFromWeb(slug)

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

	if len(parts) == 2 && parts[1] == "note" && r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		key := strings.TrimSpace(r.FormValue("key"))
		note := strings.TrimSpace(r.FormValue("note"))
		if err := updateGearNoteFromWeb(slug, key, note); err != nil {
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
		position := strings.TrimSpace(r.FormValue("position"))
		targetSection := strings.TrimSpace(r.FormValue("target_section"))
		if err := reorderGearFromWeb(slug, sourceKey, targetKey, position, targetSection); err != nil {
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

func deleteCharacterFromWeb(character string) error {
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}

	charFile := filepath.Join("data", "chars", slugifyName(character)+".yaml")
	if err := os.Remove(charFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("character does not exist")
		}
		return fmt.Errorf("delete character file: %w", err)
	}

	clearImportJob(character)
	importJobsMu.Lock()
	delete(importURLs, character)
	importJobsMu.Unlock()

	return nil
}

func addGearFromWeb(character string, gear string, merc bool) error {
	if character == "" {
		return fmt.Errorf("character cannot be empty")
	}
	if gear == "" {
		return fmt.Errorf("gear cannot be empty")
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

	entry, err := resolveGearWithLLM(gear, charClass, "", cfg)
	if err != nil {
		return fmt.Errorf("resolve gear details: %w", err)
	}
	if merc {
		entry["merc"] = true
		applyMercEtherealIfIndestructible(entry)
	}

	if strings.TrimSpace(fmt.Sprint(entry["exact_name"])) == "" {
		entry["exact_name"] = gear
	}
	if strings.TrimSpace(fmt.Sprint(entry["query"])) == "" {
		entry["query"] = gear
	}

	normalizeResolvedSlotAndRole(entry)

	gearList := coerceGearEntries(data["gear"])
	entries := cloneEntryForPossibleSlots(entry)
	if len(entries) == 0 {
		entries = []map[string]any{entry}
	}
	for _, expanded := range entries {
		if merc {
			expanded["merc"] = true
		}
		gearList = append(gearList, expanded)
	}
	setGearEntries(data, gearList)

	if err := writeCharacterData(character, data); err != nil {
		return err
	}

	return nil
}

func cloneEntryForPossibleSlots(entry map[string]any) []map[string]any {
	possible := stringSliceValue(entry["possible_slots"])
	if len(possible) == 0 {
		return nil
	}

	out := make([]map[string]any, 0, len(possible))
	for _, slot := range possible {
		norm := normalizeRunewordSlot(slot)
		if norm == "" {
			continue
		}
		copyEntry := map[string]any{}
		for k, v := range entry {
			copyEntry[k] = v
		}
		copyEntry["weapon_swap"] = false
		switch norm {
		case "offhand":
			copyEntry["slot"] = "weapon"
			copyEntry["swap_role"] = "offhand"
		case "weapon":
			copyEntry["slot"] = "weapon"
			copyEntry["swap_role"] = "main"
		case "helm", "armor", "gloves", "boots", "belt", "ring", "amulet", "inventory":
			copyEntry["slot"] = norm
			delete(copyEntry, "swap_role")
		default:
			continue
		}
		out = append(out, copyEntry)
	}
	return out
}

func hasGearEntryPlacement(entries []map[string]any, candidate map[string]any) bool {
	name := normalizeGearLookup(stringValue(candidate["exact_name"]))
	if name == "" {
		name = normalizeGearLookup(stringValue(candidate["query"]))
	}
	slot := normalizeSlotName(stringValue(candidate["slot"]))
	role := normalizeSwapRole(stringValue(candidate["swap_role"]))
	candidateMerc := isMercEntry(candidate)
	for _, existing := range entries {
		existingName := normalizeGearLookup(stringValue(existing["exact_name"]))
		if existingName == "" {
			existingName = normalizeGearLookup(stringValue(existing["query"]))
		}
		if existingName != name {
			continue
		}
		if normalizeSlotName(stringValue(existing["slot"])) != slot {
			continue
		}
		if isMercEntry(existing) != candidateMerc {
			continue
		}
		if slot == "weapon" && normalizeSwapRole(stringValue(existing["swap_role"])) != role {
			continue
		}
		return true
	}
	return false
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
	if isMercEntry(gearList[idx]) {
		return fmt.Errorf("merc weapon slots are fixed; reorder merc weapons instead")
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

func updateGearNoteFromWeb(character string, key string, note string) error {
	data, err := readCharacterData(character)
	if err != nil {
		return err
	}

	gearList := coerceGearEntries(data["gear"])
	idx, err := parseGearKey(key, len(gearList))
	if err != nil {
		return err
	}

	note = strings.TrimSpace(note)
	if note == "" {
		delete(gearList[idx], "user_note")
	} else {
		gearList[idx]["user_note"] = note
	}

	setGearEntries(data, gearList)
	return writeCharacterData(character, data)
}

func reorderGearFromWeb(character string, sourceKey string, targetKey string, position string, targetSection string) error {
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
	target := -1
	if targetKey != "" {
		target, err = parseGearKey(targetKey, len(gearList))
		if err != nil {
			return err
		}
	}

	item := gearList[source]
	if err := applyDragSectionPlacement(item, targetSection); err != nil {
		return err
	}

	gearList = append(gearList[:source], gearList[source+1:]...)

	insertAt := len(gearList)
	if target >= 0 {
		if source < target {
			target--
		}
		switch strings.ToLower(strings.TrimSpace(position)) {
		case "before":
			insertAt = target
		case "after":
			insertAt = target + 1
		default:
			insertAt = target
		}
	}
	if insertAt < 0 {
		insertAt = 0
	}
	if insertAt > len(gearList) {
		insertAt = len(gearList)
	}

	front := append([]map[string]any{}, gearList[:insertAt]...)
	front = append(front, item)
	gearList = append(front, gearList[insertAt:]...)

	setGearEntries(data, gearList)
	return writeCharacterData(character, data)
}

func applyDragSectionPlacement(entry map[string]any, section string) error {
	value := strings.ToLower(strings.TrimSpace(section))
	if value == "" {
		return nil
	}

	setMerc := func(merc bool) {
		entry["merc"] = merc
	}
	setWeapon := func(isSwap bool, role string) {
		entry["slot"] = "weapon"
		entry["weapon_swap"] = isSwap
		entry["swap_role"] = normalizeSwapRole(role)
	}
	setSimple := func(slot string) {
		entry["slot"] = slot
		entry["weapon_swap"] = false
		delete(entry, "swap_role")
	}

	switch value {
	case "weapon":
		setMerc(false)
		setWeapon(false, "main")
	case "offhand":
		setMerc(false)
		setWeapon(false, "offhand")
	case "weapon swap":
		setMerc(false)
		setWeapon(true, "main")
	case "weapon swap offhand":
		setMerc(false)
		setWeapon(true, "offhand")
	case "helm", "armor", "gloves", "boots", "belt", "ring", "amulet", "inventory":
		setMerc(false)
		setSimple(value)
	case "merc weapon":
		setMerc(true)
		setWeapon(false, "main")
	case "merc offhand":
		setMerc(true)
		setWeapon(false, "offhand")
	case "merc helm":
		setMerc(true)
		setSimple("helm")
	case "merc armor":
		setMerc(true)
		setSimple("armor")
	default:
		return fmt.Errorf("unsupported target section %q", section)
	}

	return nil
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
		for _, label := range []string{"weapon", "offhand", "weapon swap", "weapon swap offhand", "helm", "armor", "gloves", "boots", "belt", "ring", "amulet", "inventory", "merc weapon", "merc offhand", "merc helm", "merc armor"} {
			sections = append(sections, webSection{Label: label})
		}
	} else {
		bySlot := map[string][]webGearItem{}
		for _, item := range allItems {
			bySlot[item.Status.Slot] = append(bySlot[item.Status.Slot], item)
		}

		weapon := filterWebItems(bySlot["weapon"], func(s gearStatus) bool { return !s.Merc })
		mercWeapon := filterWebItems(bySlot["merc_weapon"], func(s gearStatus) bool { return s.Merc })
		mercHelm := filterWebItems(bySlot["merc_helm"], func(s gearStatus) bool { return s.Merc })
		mercArmor := filterWebItems(bySlot["merc_armor"], func(s gearStatus) bool { return s.Merc })
		mercMain, mercOffhand := splitMercWeaponSlots(mercWeapon)
		sections = append(sections, webSection{Label: "weapon", Items: filterWebItems(weapon, func(s gearStatus) bool { return !s.IsSwap && s.SwapRole != "offhand" })})
		sections = append(sections, webSection{Label: "offhand", Items: filterWebItems(weapon, func(s gearStatus) bool { return !s.IsSwap && s.SwapRole == "offhand" })})
		sections = append(sections, webSection{Label: "weapon swap", Items: filterWebItems(weapon, func(s gearStatus) bool { return s.IsSwap && s.SwapRole != "offhand" })})
		sections = append(sections, webSection{Label: "weapon swap offhand", Items: filterWebItems(weapon, func(s gearStatus) bool { return s.IsSwap && s.SwapRole == "offhand" })})

		for _, slot := range []string{"helm", "armor", "gloves", "boots", "belt", "ring", "amulet", "inventory"} {
			sections = append(sections, webSection{Label: slot, Items: filterWebItems(bySlot[slot], func(s gearStatus) bool { return !s.Merc })})
		}

		sections = append(sections, webSection{Label: "merc weapon", Items: mercMain})
		sections = append(sections, webSection{Label: "merc offhand", Items: mercOffhand})
		sections = append(sections, webSection{Label: "merc helm", Items: mercHelm})
		sections = append(sections, webSection{Label: "merc armor", Items: mercArmor})
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

func splitMercWeaponSlots(items []webGearItem) (main []webGearItem, offhand []webGearItem) {
	if len(items) == 0 {
		return nil, nil
	}

	mainCandidates := make([]webGearItem, 0, len(items))
	offhandCandidates := make([]webGearItem, 0, len(items))
	for _, item := range items {
		if normalizeSwapRole(item.Status.SwapRole) == "offhand" {
			offhandCandidates = append(offhandCandidates, item)
			continue
		}
		mainCandidates = append(mainCandidates, item)
	}

	if len(mainCandidates) > 0 {
		main = []webGearItem{mainCandidates[0]}
	}

	if len(offhandCandidates) > 0 {
		offhand = []webGearItem{offhandCandidates[0]}
	} else if len(mainCandidates) > 1 {
		offhand = []webGearItem{mainCandidates[1]}
	}

	return main, offhand
}

func applyMercEtherealIfIndestructible(entry map[string]any) {
	if strings.ToLower(strings.TrimSpace(stringValue(entry["kind"]))) != "runeword" {
		return
	}

	indestructible := hasRune(stringSliceValue(entry["runes"]), "zod")
	if !indestructible {
		for _, effect := range stringSliceValue(entry["effects"]) {
			if strings.Contains(strings.ToLower(effect), "indestruct") {
				indestructible = true
				break
			}
		}
	}
	if !indestructible {
		return
	}

	bases := stringSliceValue(entry["possible_bases"])
	details := baseDetailsSliceValue(entry["possible_bases_details"])
	bases, details = appendEtherealVariants(bases, details)
	entry["possible_bases"] = bases
	entry["possible_bases_details"] = details
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

	sort.Slice(out, func(i, j int) bool {
		if out[i].Complete != out[j].Complete {
			return !out[i].Complete
		}
		left := runeDifficultyOrder(out[i].Name)
		right := runeDifficultyOrder(out[j].Name)
		if left == right {
			return out[i].Name < out[j].Name
		}
		return left < right
	})

	return out
}
