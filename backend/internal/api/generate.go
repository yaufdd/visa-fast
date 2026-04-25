package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"fujitravel-admin/backend/internal/ai"
	dbrepo "fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/docgen"
	"fujitravel-admin/backend/internal/middleware"
)

// Sentinel errors returned by runPipeline so callers can translate them into
// HTTP status codes without string matching.
var (
	errNoTourists      = errors.New("no tourists")
	errNoHotelsForPipe = errors.New("no hotels configured")
)

// withAuditCtx wraps parent with the values that ai.callClaude uses to write
// an audit row: a fresh generation_id, the org, optional group/subgroup, and
// a Logger backed by the given pool. One generation_id per call — each
// runPipeline invocation is its own auditable unit.
func withAuditCtx(parent context.Context, pool *pgxpool.Pool, orgID, groupID, subgroupID string) context.Context {
	ctx := ai.WithGenerationID(parent, ai.NewGenerationID())
	ctx = ai.WithOrgID(ctx, orgID)
	if groupID != "" {
		ctx = ai.WithGroupID(ctx, groupID)
	}
	if subgroupID != "" {
		ctx = ai.WithSubgroupID(ctx, subgroupID)
	}
	return ai.WithLogger(ctx, dbrepo.NewPgxAILogger(pool))
}

// freeTextKeys lists the submission_snapshot fields that are free-text Russian
// strings requiring translation before they land in the final pass2 JSON.
// The set mirrors what ai.AssembleTourist feeds through the translations map.
//
// Note: home_address_ru is intentionally NOT here — it is PII and must not
// leave the server. AssembleTourist formats it locally via
// backend/internal/format + translit.RuToLatICAO.
var freeTextKeys = []string{
	"place_of_birth_ru",
	"issued_by_ru",
	"occupation_ru",
	"employer_ru",
	"employer_address_ru",
	"previous_visits_ru",
	"nationality_ru",
}

// touristRow is the minimal shape loaded from tourists for the pipeline.
type touristRow struct {
	ID           string
	GroupID      string
	SubgroupID   *string
	Payload      []byte // submission_snapshot
	FlightData   []byte
	Translations []byte
}

// loadTouristRowsForSubgroup loads all tourists for a subgroup (or the whole
// group when subgroupID == "") returning their submission_snapshot, flight_data
// and cached translations. Scoped to the calling org.
func loadTouristRowsForSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID, subgroupID string) ([]touristRow, error) {
	var rows interface {
		Next() bool
		Scan(...any) error
		Close()
	}
	var err error
	if subgroupID != "" {
		rows, err = pool.Query(ctx,
			`SELECT id, group_id, subgroup_id, submission_snapshot, flight_data, translations
			   FROM tourists
			  WHERE group_id = $1 AND subgroup_id = $2 AND org_id = $3
			  ORDER BY created_at`, groupID, subgroupID, orgID)
	} else {
		rows, err = pool.Query(ctx,
			`SELECT id, group_id, subgroup_id, submission_snapshot, flight_data, translations
			   FROM tourists
			  WHERE group_id = $1 AND subgroup_id IS NULL AND org_id = $2
			  ORDER BY created_at`, groupID, orgID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []touristRow
	for rows.Next() {
		var t touristRow
		var subID *string
		var payload, flight, tr []byte
		if err := rows.Scan(&t.ID, &t.GroupID, &subID, &payload, &flight, &tr); err != nil {
			return nil, err
		}
		t.SubgroupID = subID
		t.Payload = payload
		t.FlightData = flight
		t.Translations = tr
		out = append(out, t)
	}
	return out, nil
}

// loadHotelBriefsForSubgroup loads hotels for a specific subgroup as ai.HotelBrief
// entries (the shape consumed by the new pipeline). If subgroupID is "",
// it falls back to all hotels for the group (legacy/no-subgroup mode).
// Scoped to the calling org via group_hotels.org_id.
func loadHotelBriefsForSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID, subgroupID string) ([]ai.HotelBrief, error) {
	var hRows interface {
		Next() bool
		Scan(...any) error
		Close()
	}
	var err error
	if subgroupID != "" {
		hRows, err = pool.Query(ctx,
			`SELECT h.name_en, h.city, COALESCE(h.address,''), COALESCE(h.phone,''),
			        gh.check_in::text, gh.check_out::text
			   FROM group_hotels gh
			   JOIN hotels h ON h.id = gh.hotel_id
			  WHERE gh.subgroup_id = $1 AND gh.org_id = $2
			  ORDER BY gh.sort_order`, subgroupID, orgID)
	} else {
		hRows, err = pool.Query(ctx,
			`SELECT h.name_en, h.city, COALESCE(h.address,''), COALESCE(h.phone,''),
			        gh.check_in::text, gh.check_out::text
			   FROM group_hotels gh
			   JOIN hotels h ON h.id = gh.hotel_id
			  WHERE gh.group_id = $1 AND gh.subgroup_id IS NULL AND gh.org_id = $2
			  ORDER BY gh.sort_order`, groupID, orgID)
	}
	if err != nil {
		return nil, err
	}
	defer hRows.Close()
	var hotels []ai.HotelBrief
	for hRows.Next() {
		var h ai.HotelBrief
		if err := hRows.Scan(&h.Name, &h.City, &h.Address, &h.Phone, &h.CheckIn, &h.CheckOut); err != nil {
			return nil, err
		}
		hotels = append(hotels, h)
	}
	return hotels, nil
}

// touristPayloads unmarshals each tourist's submission_snapshot into map[string]any.
// Missing / invalid payloads become empty maps so downstream indexing is safe.
func touristPayloads(rows []touristRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		m := map[string]any{}
		if len(r.Payload) > 0 {
			_ = json.Unmarshal(r.Payload, &m)
		}
		out[i] = m
	}
	return out
}

// touristFlights unmarshals each tourist's flight_data into map[string]any.
func touristFlights(rows []touristRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		m := map[string]any{}
		if len(r.FlightData) > 0 {
			_ = json.Unmarshal(r.FlightData, &m)
		}
		out[i] = m
	}
	return out
}

// collectFreeText walks the tourist payloads collecting distinct non-empty
// values for freeTextKeys. It returns the unique strings (stable order) and a
// per-tourist map from source string → index in the uniques slice so the
// translation output can be indexed back to each tourist.
func collectFreeText(payloads []map[string]any, keys []string) ([]string, []map[string]int) {
	uniques := make([]string, 0, 16)
	indexOfUnique := map[string]int{}
	perTourist := make([]map[string]int, len(payloads))

	for i, payload := range payloads {
		perTourist[i] = map[string]int{}
		for _, k := range keys {
			v, ok := payload[k].(string)
			if !ok || v == "" {
				continue
			}
			idx, seen := indexOfUnique[v]
			if !seen {
				idx = len(uniques)
				uniques = append(uniques, v)
				indexOfUnique[v] = idx
			}
			perTourist[i][v] = idx
		}
	}
	return uniques, perTourist
}

// buildTranslationMaps turns the batched translator output into per-tourist
// source → translated maps suitable for ai.AssembleTourist.
func buildTranslationMaps(perTourist []map[string]int, translations []string) []map[string]string {
	out := make([]map[string]string, len(perTourist))
	for i, m := range perTourist {
		tm := make(map[string]string, len(m))
		for src, idx := range m {
			if idx < 0 || idx >= len(translations) {
				continue
			}
			t := translations[idx]
			if t == "" {
				continue
			}
			tm[src] = t
		}
		out[i] = tm
	}
	return out
}

// buildProgrammeInput picks the first tourist with a populated outbound flight
// as the "lead ticket" (programme shows one shared activity cell) and packages
// the non-PII trip data needed by ai.GenerateProgramme.
func buildProgrammeInput(payloads []map[string]any, flights []map[string]any, hotels []ai.HotelBrief, contactPhone, managerNotes string) ai.ProgrammeInput {
	var in ai.ProgrammeInput
	in.Hotels = hotels
	in.ContactPhone = contactPhone
	in.ManagerNotes = managerNotes

	for i, fl := range flights {
		arr := subMapAny(fl, "arrival")
		dep := subMapAny(fl, "departure")
		if strGetAny(arr, "flight_number") == "" {
			continue
		}
		in.ArrivalDate = strGetAny(arr, "date")
		in.ArrivalFlight = ai.FlightBrief{
			Number:  strGetAny(arr, "flight_number"),
			Time:    strGetAny(arr, "time"),
			Airport: strGetAny(arr, "airport"),
		}
		if strGetAny(dep, "flight_number") != "" {
			in.DepartureDate = strGetAny(dep, "date")
			in.DepartureFlight = ai.FlightBrief{
				Number:  strGetAny(dep, "flight_number"),
				Time:    strGetAny(dep, "time"),
				Airport: strGetAny(dep, "airport"),
			}
		}
		// Fallback contact phone: use lead tourist's phone if none supplied.
		if in.ContactPhone == "" && i < len(payloads) {
			if phone, ok := payloads[i]["phone"].(string); ok {
				in.ContactPhone = phone
			}
		}
		return in
	}

	// No tourist has an outbound flight — still return whatever we have.
	if in.ContactPhone == "" && len(payloads) > 0 {
		if phone, ok := payloads[0]["phone"].(string); ok {
			in.ContactPhone = phone
		}
	}
	return in
}

// runPipeline executes the NEW AI pipeline for one subgroup (or whole group if
// subgroupID=="") and returns the marshaled pass2 JSON bytes.
//
// translator is the Yandex-backed Translator used for translate.go's batch
// of dry Russian → English fields; apiKey is the Anthropic key still used
// by the not-yet-migrated programme path. Once tasks 1.B2 / 1.C* land,
// apiKey will be replaced by Yandex-shaped clients for those paths too.
func runPipeline(ctx context.Context, pool *pgxpool.Pool, translator ai.Translator, apiKey, orgID, groupID, subgroupID, guidePhone string, now time.Time) ([]byte, error) {
	rows, err := loadTouristRowsForSubgroup(ctx, pool, orgID, groupID, subgroupID)
	if err != nil {
		return nil, fmt.Errorf("load tourists: %w", err)
	}
	if len(rows) == 0 {
		return nil, errNoTourists
	}

	hotels, err := loadHotelBriefsForSubgroup(ctx, pool, orgID, groupID, subgroupID)
	if err != nil {
		return nil, fmt.Errorf("load hotels: %w", err)
	}
	if len(hotels) == 0 {
		return nil, errNoHotelsForPipe
	}

	payloads := touristPayloads(rows)
	flights := touristFlights(rows)

	// Manager-authored programme hints. Subgroup-level notes take priority
	// (each subgroup typically has its own itinerary); the group-level
	// field is kept as a legacy fallback for cases without subgroups.
	var programmeNotes string
	if subgroupID != "" {
		if sgNotes, err := dbrepo.GetSubgroupProgrammeNotes(ctx, pool, orgID, subgroupID); err == nil && sgNotes != nil {
			programmeNotes = *sgNotes
		}
	}
	if programmeNotes == "" {
		if g, err := dbrepo.GetGroup(ctx, pool, orgID, groupID); err == nil && g != nil && g.ProgrammeNotes != nil {
			programmeNotes = *g.ProgrammeNotes
		}
	}

	uniques, perTourist := collectFreeText(payloads, freeTextKeys)
	programmeInput := buildProgrammeInput(payloads, flights, hotels, guidePhone, programmeNotes)

	// Doverenost free-text fields (internal_issued_by_ru, reg_address_ru) are
	// formatted LOCALLY by ai.AssembleDoverenost via backend/internal/format.
	// They used to go through Claude Haiku (CleanDoverenostFields) — removed
	// because these fields are PII-adjacent and must not leave the server.

	var (
		translations []string
		programme    []ai.ProgrammeDay
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		t, err := ai.TranslateStrings(gctx, translator, uniques)
		if err != nil {
			return fmt.Errorf("translate: %w", err)
		}
		translations = t
		return nil
	})
	g.Go(func() error {
		p, err := ai.GenerateProgramme(gctx, translator, programmeInput)
		if err != nil {
			return fmt.Errorf("programme: %w", err)
		}
		programme = p
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	trMaps := buildTranslationMaps(perTourist, translations)

	// Best-effort cache of per-tourist translations back into DB.
	for i, row := range rows {
		if i >= len(trMaps) {
			break
		}
		buf, mErr := json.Marshal(trMaps[i])
		if mErr != nil {
			continue
		}
		if _, err := pool.Exec(ctx,
			`UPDATE tourists SET translations = $1, updated_at = NOW()
			  WHERE id = $2 AND org_id = $3`,
			buf, row.ID, orgID); err != nil {
			slog.Warn("cache tourist translations", "tourist_id", row.ID, "err", err)
		}
	}

	pass2 := ai.AssemblePass2(
		payloads,
		trMaps,
		flights,
		programme,
		hotels,
		now.Format("02.01.2006"),
	)

	out, err := json.MarshalIndent(pass2, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal pass2: %w", err)
	}
	return out, nil
}

// GenerateDocuments handles POST /api/groups/:id/generate
// Supports subgroups: each subgroup gets its own folder in output.zip.
func GenerateDocuments(pool *pgxpool.Pool, translator ai.Translator, apiKey, uploadsDir, pythonScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		g, err := dbrepo.GetGroup(r.Context(), pool, orgID, groupID)
		if err != nil {
			slog.Error("get group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if g == nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		groupName := g.Name

		// Load subgroups scoped to the org.
		sgRows, err := pool.Query(r.Context(),
			`SELECT id, name FROM subgroups
			   WHERE group_id = $1 AND org_id = $2
			   ORDER BY sort_order, created_at`, groupID, orgID)
		if err != nil {
			slog.Error("fetch subgroups", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		type subgroupEntry struct{ id, name string }
		var subgroups []subgroupEntry
		for sgRows.Next() {
			var sg subgroupEntry
			if err := sgRows.Scan(&sg.id, &sg.name); err == nil {
				subgroups = append(subgroups, sg)
			}
		}
		sgRows.Close()

		// If no subgroups defined, treat whole group as one subgroup named after the group.
		if len(subgroups) == 0 {
			subgroups = []subgroupEntry{{id: "", name: groupName}}
		}

		guidePhone := r.URL.Query().Get("guide_phone")
		now := time.Now()

		// Clean docs dir before generating.
		docsDir := filepath.Join(uploadsDir, groupID, "docs")
		_ = os.RemoveAll(docsDir)

		var lastPass2JSON []byte

		for _, sg := range subgroups {
			aiCtx := withAuditCtx(r.Context(), pool, orgID, groupID, sg.id)
			pass2JSON, err := runPipeline(aiCtx, pool, translator, apiKey, orgID, groupID, sg.id, guidePhone, now)
			if err != nil {
				if errors.Is(err, errNoTourists) {
					slog.Warn("subgroup has no tourists, skipping", "subgroup", sg.name)
					continue
				}
				if errors.Is(err, errNoHotelsForPipe) {
					writeError(w, http.StatusBadRequest, "no hotels configured for subgroup "+sg.name)
					return
				}
				slog.Error("ai pipeline", "subgroup", sg.name, "err", err)
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("ai pipeline (%s): %s", sg.name, err))
				return
			}
			lastPass2JSON = pass2JSON

			if err := docgen.GenerateWithSubgroup(r.Context(), pythonScript, uploadsDir, orgID, groupID, sg.name, pass2JSON); err != nil {
				slog.Error("docgen subgroup", "subgroup", sg.name, "err", err)
				writeError(w, http.StatusInternalServerError, "document generation failed for "+sg.name+": "+err.Error())
				return
			}
		}

		if lastPass2JSON == nil {
			writeError(w, http.StatusBadRequest, "no tourists found in any subgroup")
			return
		}

		// Pack all docs/{subgroup}/ folders into one ZIP.
		zipPath, err := docgen.ZipDocsDir(uploadsDir, groupID, "output.zip")
		if err != nil {
			slog.Error("zip docs", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to create zip: "+err.Error())
			return
		}

		// Save last pass2_json (for finalize step).
		docID, err := dbrepo.CreateDocument(r.Context(), pool, orgID, groupID, zipPath, lastPass2JSON, now)
		if err != nil {
			slog.Error("insert document", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"document_id":  docID,
			"zip_path":     zipPath,
			"generated_at": now,
		})
	}
}

// GenerateSubgroupDocuments handles POST /api/subgroups/:id/generate
// Generates docs for a single subgroup and returns a per-subgroup ZIP.
func GenerateSubgroupDocuments(pool *pgxpool.Pool, translator ai.Translator, apiKey, uploadsDir, pythonScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		subgroupID := chi.URLParam(r, "id")

		var groupID, subgroupName string
		if err := pool.QueryRow(r.Context(),
			`SELECT group_id, name FROM subgroups WHERE id = $1 AND org_id = $2`,
			subgroupID, orgID,
		).Scan(&groupID, &subgroupName); err != nil {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}
		// Reject path separators / traversal in subgroup name to keep os.RemoveAll
		// and the Python docgen confined to the intended subfolder.
		if strings.ContainsAny(subgroupName, "/\\") || strings.Contains(subgroupName, "..") {
			writeError(w, http.StatusBadRequest, "invalid subgroup name — remove / \\ ..")
			return
		}

		guidePhone := r.URL.Query().Get("guide_phone")
		now := time.Now()

		// Clean only this subgroup's docs subfolder.
		subDocsDir := filepath.Join(uploadsDir, groupID, "docs", subgroupName)
		_ = os.RemoveAll(subDocsDir)

		aiCtx := withAuditCtx(r.Context(), pool, orgID, groupID, subgroupID)
		pass2JSON, err := runPipeline(aiCtx, pool, translator, apiKey, orgID, groupID, subgroupID, guidePhone, now)
		if err != nil {
			if errors.Is(err, errNoTourists) {
				writeError(w, http.StatusBadRequest, "no tourists in this subgroup")
				return
			}
			if errors.Is(err, errNoHotelsForPipe) {
				writeError(w, http.StatusBadRequest, "no hotels configured for this subgroup")
				return
			}
			slog.Error("ai pipeline subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "AI pipeline failed: "+err.Error())
			return
		}

		if err := docgen.GenerateWithSubgroup(r.Context(), pythonScript, uploadsDir, orgID, groupID, subgroupName, pass2JSON); err != nil {
			slog.Error("docgen subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "document generation failed: "+err.Error())
			return
		}

		zipName := "subgroup_" + subgroupID + ".zip"
		zipPath, err := docgen.ZipSubgroupDir(uploadsDir, groupID, subgroupName, zipName)
		if err != nil {
			slog.Error("zip subgroup docs", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to create zip: "+err.Error())
			return
		}

		// Save pass2_json so finalize can use it later.
		if _, err := dbrepo.CreateDocument(r.Context(), pool, orgID, groupID, zipPath, pass2JSON, now); err != nil {
			slog.Warn("insert document record", "err", err)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"subgroup_id":  subgroupID,
			"zip_path":     zipPath,
			"generated_at": now,
		})
	}
}

// DownloadSubgroupZIP handles GET /api/subgroups/:id/download
func DownloadSubgroupZIP(pool *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		subgroupID := chi.URLParam(r, "id")
		var groupID, subgroupName string
		if err := pool.QueryRow(r.Context(),
			`SELECT group_id, name FROM subgroups WHERE id = $1 AND org_id = $2`,
			subgroupID, orgID,
		).Scan(&groupID, &subgroupName); err != nil {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}
		zipPath := filepath.Join(uploadsDir, groupID, "subgroup_"+subgroupID+".zip")
		if _, err := os.Stat(zipPath); err != nil {
			writeError(w, http.StatusNotFound, "no documents generated for this subgroup")
			return
		}
		// Strip characters that would break Content-Disposition header (quotes, newlines).
		safeName := strings.Map(func(r rune) rune {
			if r == '"' || r == '\r' || r == '\n' {
				return -1
			}
			return r
		}, subgroupName)
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+safeName+`.zip"`)
		http.ServeFile(w, r, zipPath)
	}
}

// isCyrillic returns true if the string contains Cyrillic letters.
func isCyrillic(s string) bool {
	for _, r := range s {
		if r >= 0x0400 && r <= 0x04FF {
			return true
		}
	}
	return false
}

// normalizeNameKey builds a dedupe key from a Cyrillic name: lowercase,
// strip spaces and non-letters. Lets minor typos be caught by editDistance.
func normalizeNameKey(s string) string {
	var out []rune
	for _, r := range strings.ToLower(s) {
		if (r >= 'а' && r <= 'я') || r == 'ё' {
			out = append(out, r)
		}
	}
	return string(out)
}

// editDistance returns the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = prev[j] + 1
			if curr[j-1]+1 < curr[j] {
				curr[j] = curr[j-1] + 1
			}
			if prev[j-1]+cost < curr[j] {
				curr[j] = prev[j-1] + cost
			}
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

// translitLatToCyr converts a Latin passport-style name to Cyrillic
// (uppercase input assumed). Simple deterministic mapping — good enough for
// Russian passport transliteration used in the "для Инны" list.
// If the input already contains Cyrillic, returns it title-cased as-is.
func translitLatToCyr(s string) string {
	if isCyrillic(s) {
		// Title-case each word.
		words := strings.Fields(s)
		for i, w := range words {
			runes := []rune(w)
			if len(runes) > 0 {
				runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
				for j := 1; j < len(runes); j++ {
					runes[j] = []rune(strings.ToLower(string(runes[j])))[0]
				}
			}
			words[i] = string(runes)
		}
		return strings.Join(words, " ")
	}
	// Multi-char sequences first, then single chars.
	replacements := []struct{ from, to string }{
		{"SHCH", "Щ"}, {"YO", "Ё"}, {"YU", "Ю"}, {"YA", "Я"},
		{"ZH", "Ж"}, {"KH", "Х"}, {"TS", "Ц"}, {"CH", "Ч"}, {"SH", "Ш"},
		{"IE", "ИЕ"},
	}
	upper := strings.ToUpper(s)
	for _, rr := range replacements {
		upper = strings.ReplaceAll(upper, rr.from, rr.to)
	}
	single := map[rune]rune{
		'A': 'А', 'B': 'Б', 'V': 'В', 'G': 'Г', 'D': 'Д', 'E': 'Е',
		'Z': 'З', 'I': 'И', 'Y': 'Й', 'K': 'К', 'L': 'Л', 'M': 'М',
		'N': 'Н', 'O': 'О', 'P': 'П', 'R': 'Р', 'S': 'С', 'T': 'Т',
		'U': 'У', 'F': 'Ф',
	}
	var out []rune
	for _, r := range upper {
		if c, ok := single[r]; ok {
			out = append(out, c)
		} else {
			out = append(out, r)
		}
	}
	// Title-case each word: first letter upper, rest lower.
	words := strings.Fields(string(out))
	for i, w := range words {
		runes := []rune(w)
		for j := 1; j < len(runes); j++ {
			runes[j] = []rune(strings.ToLower(string(runes[j])))[0]
		}
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

// FinalizeGroup handles POST /api/groups/:id/finalize
// Generates group-level docs (для Инны в ВЦ, заявка ВЦ) for ALL tourists across
// ALL subgroups in the подача. Skips AI — builds the list from DB directly
// and reuses the latest subgroup's pass2_json as a template for trip-level fields.
func FinalizeGroup(pool *pgxpool.Pool, apiKey, uploadsDir, pythonScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		// Ensure the group belongs to this org before doing anything else.
		g, err := dbrepo.GetGroup(r.Context(), pool, orgID, groupID)
		if err != nil {
			slog.Error("get group", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if g == nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		// Load the last saved pass2_json for this group — used as a template.
		latest, err := dbrepo.LatestDocumentForGroup(r.Context(), pool, orgID, groupID)
		if err != nil || latest == nil {
			writeError(w, http.StatusBadRequest, "no generated documents found — сначала сгенерируйте документы хотя бы для одной группы")
			return
		}
		templateJSON := []byte(latest.Pass2JSON)

		// Collect Russian names of ALL tourists across ALL subgroups from submission_snapshot.
		tourists, err := dbrepo.ListTouristsByGroup(r.Context(), pool, orgID, groupID)
		if err != nil {
			slog.Error("fetch tourists for finalize", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		// Two-pass: collect candidates with a "reliability score".
		// 2 = name_cyr from submission (reliable), 0 = transliterated from name_lat.
		type candidate struct {
			name  string
			score int
		}
		var candidates []candidate
		for _, t := range tourists {
			snapshot := []byte(t.SubmissionSnapshot)
			if len(snapshot) == 0 {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal(snapshot, &payload); err != nil {
				continue
			}
			name := ""
			score := 0
			if s, ok := payload["name_cyr"].(string); ok && s != "" {
				name = s
				score = 2
			}
			if name == "" {
				if s, ok := payload["name_lat"].(string); ok && s != "" {
					name = translitLatToCyr(s)
					score = 0
				}
			}
			if name == "" {
				continue
			}
			candidates = append(candidates, candidate{name: name, score: score})
		}

		// Fuzzy dedupe preserving creation order. When a duplicate is found (edit
		// distance ≤ 2 on normalized key), keep the version with the higher score.
		type kept struct {
			name  string
			key   string
			score int
		}
		var keptList []kept
		for _, c := range candidates {
			key := normalizeNameKey(c.name)
			if key == "" {
				continue
			}
			found := -1
			for i, k := range keptList {
				if editDistance(key, k.key) <= 2 {
					found = i
					break
				}
			}
			if found >= 0 {
				if c.score > keptList[found].score {
					keptList[found] = kept{name: c.name, key: key, score: c.score}
				}
				continue
			}
			keptList = append(keptList, kept{name: c.name, key: key, score: c.score})
		}
		applicantsRu := make([]string, 0, len(keptList))
		for _, k := range keptList {
			applicantsRu = append(applicantsRu, k.name)
		}

		if len(applicantsRu) == 0 {
			writeError(w, http.StatusBadRequest, "no tourists with submission data found in this group")
			return
		}

		// Resolve submission date: ?submission_date=YYYY-MM-DD from query, else tomorrow.
		submissionDateDDMMYYYY := ""
		if raw := r.URL.Query().Get("submission_date"); raw != "" {
			if t, err := time.Parse("2006-01-02", raw); err == nil {
				submissionDateDDMMYYYY = t.Format("02.01.2006")
			}
		}
		if submissionDateDDMMYYYY == "" {
			submissionDateDDMMYYYY = time.Now().AddDate(0, 0, 1).Format("02.01.2006")
		}

		// Patch the template JSON: override inna_doc.applicants_ru, vc_request, and submission date.
		var doc map[string]any
		if err := json.Unmarshal(templateJSON, &doc); err != nil {
			slog.Error("unmarshal template pass2", "err", err)
			writeError(w, http.StatusInternalServerError, "corrupt template JSON")
			return
		}
		if inna, ok := doc["inna_doc"].(map[string]any); ok {
			inna["applicants_ru"] = applicantsRu
			inna["submission_date"] = submissionDateDDMMYYYY
			doc["inna_doc"] = inna
		} else {
			doc["inna_doc"] = map[string]any{
				"applicants_ru":   applicantsRu,
				"submission_date": submissionDateDDMMYYYY,
			}
		}
		// Override arrival.date so the заявка ВЦ filename reflects the submission date
		// ("на DD месяц N.docx" is built from arrival.date in the Python script).
		if arr, ok := doc["arrival"].(map[string]any); ok {
			arr["date"] = submissionDateDDMMYYYY
			doc["arrival"] = arr
		} else {
			doc["arrival"] = map[string]any{"date": submissionDateDDMMYYYY}
		}
		if vc, ok := doc["vc_request"].(map[string]any); ok {
			vc["applicants"] = applicantsRu
			vc["count"] = len(applicantsRu)
			vc["service_fee_per_person"] = 970
			vc["service_fee_total"] = 970 * len(applicantsRu)
			doc["vc_request"] = vc
		} else {
			doc["vc_request"] = map[string]any{
				"applicants": applicantsRu, "count": len(applicantsRu),
				"service_fee_per_person": 970, "service_fee_total": 970 * len(applicantsRu),
			}
		}

		mergedJSON, err := json.Marshal(doc)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "marshal merged JSON")
			return
		}

		zipPath, err := docgen.GenerateFinal(r.Context(), pythonScript, uploadsDir, orgID, groupID, mergedJSON)
		if err != nil {
			slog.Error("docgen final", "err", err)
			writeError(w, http.StatusInternalServerError, "final document generation failed: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"zip_path": zipPath, "count": len(applicantsRu)})
	}
}

// GetDocuments handles GET /api/groups/:id/documents
func GetDocuments(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		docs, err := dbrepo.ListDocumentsForGroup(r.Context(), pool, orgID, groupID)
		if err != nil {
			slog.Error("fetch documents", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusOK, docs)
	}
}

// FinalStatus handles GET /api/groups/:id/final/status
// Returns whether the final.zip exists on disk and when it was generated.
// Scoped to org: groups from other tenants return has_zip=false even if a
// file happens to exist on disk under that UUID.
func FinalStatus(pool *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")
		resp := map[string]any{"has_zip": false}

		g, err := dbrepo.GetGroup(r.Context(), pool, orgID, groupID)
		if err != nil || g == nil {
			writeJSON(w, http.StatusOK, resp)
			return
		}

		zipPath := filepath.Join(uploadsDir, groupID, "final.zip")
		if info, err := os.Stat(zipPath); err == nil && !info.IsDir() {
			resp["has_zip"] = true
			resp["generated_at"] = info.ModTime()
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// DownloadFinalZIP handles GET /api/groups/:id/download/final
func DownloadFinalZIP(pool *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		g, err := dbrepo.GetGroup(r.Context(), pool, orgID, groupID)
		if err != nil || g == nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		finalPath := filepath.Join(uploadsDir, groupID, "final.zip")
		if _, err := os.Stat(finalPath); err != nil {
			writeError(w, http.StatusNotFound, "final.zip not found — run finalize first")
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="final.zip"`)
		http.ServeFile(w, r, finalPath)
	}
}

// DownloadZIP handles GET /api/groups/:id/download
func DownloadZIP(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groupID := chi.URLParam(r, "id")

		latest, err := dbrepo.LatestDocumentForGroup(r.Context(), pool, orgID, groupID)
		if err != nil || latest == nil {
			writeError(w, http.StatusNotFound, "no documents found for this group")
			return
		}
		zipPath := latest.ZipPath

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(zipPath)+`"`)
		http.ServeFile(w, r, zipPath)
	}
}

// subMapAny extracts a nested map[string]any from m[key].
func subMapAny(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key]; ok {
		if sub, ok := v.(map[string]any); ok {
			return sub
		}
	}
	return nil
}

// strGetAny returns m[k] as a string (empty if missing or wrong type).
func strGetAny(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
