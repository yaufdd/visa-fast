package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/ai"
	"fujitravel-admin/backend/internal/docgen"
)

// loadHotelsForSubgroup loads hotels for a specific subgroup. If subgroupID is "",
// it falls back to all hotels for the group (legacy/no-subgroup mode).
func loadHotelsForSubgroup(db *pgxpool.Pool, r *http.Request, groupID, subgroupID string) ([]ai.HotelEntry, error) {
	var hRows interface {
		Next() bool
		Scan(...any) error
		Close()
	}
	var err error
	if subgroupID != "" {
		hRows, err = db.Query(r.Context(),
			`SELECT h.name_en, COALESCE(h.address,''), COALESCE(h.phone,''), h.city,
			        gh.check_in::text, gh.check_out::text, COALESCE(gh.room_type,''), gh.sort_order
			   FROM group_hotels gh
			   JOIN hotels h ON h.id = gh.hotel_id
			  WHERE gh.subgroup_id = $1
			  ORDER BY gh.sort_order`, subgroupID)
	} else {
		hRows, err = db.Query(r.Context(),
			`SELECT h.name_en, COALESCE(h.address,''), COALESCE(h.phone,''), h.city,
			        gh.check_in::text, gh.check_out::text, COALESCE(gh.room_type,''), gh.sort_order
			   FROM group_hotels gh
			   JOIN hotels h ON h.id = gh.hotel_id
			  WHERE gh.group_id = $1 AND gh.subgroup_id IS NULL
			  ORDER BY gh.sort_order`, groupID)
	}
	if err != nil {
		return nil, err
	}
	defer hRows.Close()
	var hotels []ai.HotelEntry
	for hRows.Next() {
		var h ai.HotelEntry
		if err := hRows.Scan(&h.NameEn, &h.Address, &h.Phone, &h.City,
			&h.CheckIn, &h.CheckOut, &h.RoomType, &h.SortOrder); err != nil {
			return nil, err
		}
		hotels = append(hotels, h)
	}
	return hotels, nil
}

// loadTouristsForSubgroup loads confirmed tourists for a subgroup (or whole group if subgroupID=="").
func loadTouristsForSubgroup(db *pgxpool.Pool, r *http.Request, groupID, subgroupID string) ([]ai.TouristData, error) {
	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var err error
	if subgroupID != "" {
		rows, err = db.Query(r.Context(),
			`SELECT raw_json, matched_sheet_row FROM tourists
			  WHERE group_id = $1 AND subgroup_id = $2 AND match_confirmed = true
			  ORDER BY created_at`, groupID, subgroupID)
	} else {
		rows, err = db.Query(r.Context(),
			`SELECT raw_json, matched_sheet_row FROM tourists
			  WHERE group_id = $1 AND match_confirmed = true
			  ORDER BY created_at`, groupID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tourists []ai.TouristData
	for rows.Next() {
		var rawJSON, matchedRow []byte
		if err := rows.Scan(&rawJSON, &matchedRow); err != nil {
			return nil, err
		}
		t := ai.TouristData{RawJSON: json.RawMessage(rawJSON)}
		if matchedRow != nil {
			var m map[string]string
			_ = json.Unmarshal(matchedRow, &m)
			t.MatchedSheetRow = m
		}
		tourists = append(tourists, t)
	}
	return tourists, nil
}

// GenerateDocuments handles POST /api/groups/:id/generate
// Supports subgroups: each subgroup gets its own folder in output.zip.
func GenerateDocuments(db *pgxpool.Pool, apiKey, uploadsDir, pythonScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		var groupName string
		if err := db.QueryRow(r.Context(), `SELECT name FROM groups WHERE id = $1`, groupID).Scan(&groupName); err != nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		// Load subgroups.
		sgRows, err := db.Query(r.Context(),
			`SELECT id, name FROM subgroups WHERE group_id = $1 ORDER BY sort_order, created_at`, groupID)
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

		var lastPass2JSON json.RawMessage

		for _, sg := range subgroups {
			tourists, err := loadTouristsForSubgroup(db, r, groupID, sg.id)
			if err != nil {
				slog.Error("fetch tourists for subgroup", "subgroup", sg.name, "err", err)
				writeError(w, http.StatusInternalServerError, "database error")
				return
			}
			if len(tourists) == 0 {
				slog.Warn("subgroup has no confirmed tourists, skipping", "subgroup", sg.name)
				continue
			}

			// Load hotels specific to this subgroup.
			hotels, err := loadHotelsForSubgroup(db, r, groupID, sg.id)
			if err != nil {
				slog.Error("fetch hotels for subgroup", "subgroup", sg.name, "err", err)
				writeError(w, http.StatusInternalServerError, "database error")
				return
			}
			if len(hotels) == 0 {
				writeError(w, http.StatusBadRequest, "no hotels configured for subgroup "+sg.name)
				return
			}

			pass2Input := ai.Pass2Input{
				Tourists:   tourists,
				Hotels:     hotels,
				GuidePhone: guidePhone,
				TodayDate:  now.Format("02.01.2006"),
			}

			pass2JSON, err := ai.FormatDocuments(r.Context(), apiKey, pass2Input)
			if err != nil {
				slog.Error("ai pass2", "subgroup", sg.name, "err", err)
				writeError(w, http.StatusInternalServerError, "AI Pass 2 failed for "+sg.name+": "+err.Error())
				return
			}
			lastPass2JSON = pass2JSON

			if err := docgen.GenerateWithSubgroup(r.Context(), pythonScript, uploadsDir, groupID, sg.name, pass2JSON); err != nil {
				slog.Error("docgen subgroup", "subgroup", sg.name, "err", err)
				writeError(w, http.StatusInternalServerError, "document generation failed for "+sg.name+": "+err.Error())
				return
			}
		}

		// Pack all docs/{subgroup}/ folders into one ZIP.
		zipPath, err := docgen.ZipDocsDir(uploadsDir, groupID, "output.zip")
		if err != nil {
			slog.Error("zip docs", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to create zip: "+err.Error())
			return
		}

		// Save last pass2_json (for finalize step).
		var docID string
		err = db.QueryRow(r.Context(),
			`INSERT INTO documents (group_id, pass2_json, zip_path, generated_at)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			groupID, []byte(lastPass2JSON), zipPath, now,
		).Scan(&docID)
		if err != nil {
			slog.Error("insert document", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		if _, err := db.Exec(r.Context(),
			`UPDATE groups SET status = 'completed', updated_at = now() WHERE id = $1`, groupID); err != nil {
			slog.Error("update group completed", "err", err)
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
func GenerateSubgroupDocuments(db *pgxpool.Pool, apiKey, uploadsDir, pythonScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subgroupID := chi.URLParam(r, "id")

		var groupID, subgroupName string
		if err := db.QueryRow(r.Context(),
			`SELECT group_id, name FROM subgroups WHERE id = $1`, subgroupID).Scan(&groupID, &subgroupName); err != nil {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}

		tourists, err := loadTouristsForSubgroup(db, r, groupID, subgroupID)
		if err != nil {
			slog.Error("fetch tourists for subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if len(tourists) == 0 {
			writeError(w, http.StatusBadRequest, "no confirmed tourists in this subgroup")
			return
		}

		hotels, err := loadHotelsForSubgroup(db, r, groupID, subgroupID)
		if err != nil {
			slog.Error("fetch hotels for subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if len(hotels) == 0 {
			writeError(w, http.StatusBadRequest, "no hotels configured for this subgroup")
			return
		}

		guidePhone := r.URL.Query().Get("guide_phone")
		now := time.Now()

		// Clean only this subgroup's docs subfolder.
		subDocsDir := filepath.Join(uploadsDir, groupID, "docs", subgroupName)
		_ = os.RemoveAll(subDocsDir)

		pass2Input := ai.Pass2Input{
			Tourists:   tourists,
			Hotels:     hotels,
			GuidePhone: guidePhone,
			TodayDate:  now.Format("02.01.2006"),
		}

		pass2JSON, err := ai.FormatDocuments(r.Context(), apiKey, pass2Input)
		if err != nil {
			slog.Error("ai pass2 subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "AI Pass 2 failed: "+err.Error())
			return
		}

		if err := docgen.GenerateWithSubgroup(r.Context(), pythonScript, uploadsDir, groupID, subgroupName, pass2JSON); err != nil {
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
		if _, err := db.Exec(r.Context(),
			`INSERT INTO documents (group_id, pass2_json, zip_path, generated_at)
			 VALUES ($1, $2, $3, $4)`,
			groupID, []byte(pass2JSON), zipPath, now,
		); err != nil {
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
func DownloadSubgroupZIP(db *pgxpool.Pool, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subgroupID := chi.URLParam(r, "id")
		var groupID, subgroupName string
		if err := db.QueryRow(r.Context(),
			`SELECT group_id, name FROM subgroups WHERE id = $1`, subgroupID).Scan(&groupID, &subgroupName); err != nil {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}
		zipPath := filepath.Join(uploadsDir, groupID, "subgroup_"+subgroupID+".zip")
		if _, err := os.Stat(zipPath); err != nil {
			writeError(w, http.StatusNotFound, "no documents generated for this subgroup")
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+subgroupName+`.zip"`)
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
// ALL subgroups in the подача. Skips Pass 2 — builds the list from DB directly
// and reuses the latest subgroup's pass2_json as a template for trip-level fields.
func FinalizeGroup(db *pgxpool.Pool, apiKey, uploadsDir, pythonScript string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		// Load the last saved pass2_json for this group — used as a template.
		var templateJSON []byte
		if err := db.QueryRow(r.Context(),
			`SELECT pass2_json FROM documents WHERE group_id = $1 ORDER BY generated_at DESC LIMIT 1`,
			groupID,
		).Scan(&templateJSON); err != nil {
			writeError(w, http.StatusBadRequest, "no generated documents found — сначала сгенерируйте документы хотя бы для одной группы")
			return
		}

		// Collect Russian names of ALL tourists across ALL subgroups (no match_confirmed filter).
		rows, err := db.Query(r.Context(),
			`SELECT raw_json, matched_sheet_row FROM tourists
			  WHERE group_id = $1
			  ORDER BY created_at`, groupID)
		if err != nil {
			slog.Error("fetch tourists for finalize", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		// Two-pass: collect candidates with a "reliability score" (parsed > sheet).
		type candidate struct {
			name  string
			score int // 2 = from raw_json.name_cyr, 1 = from sheet Cyrillic, 0 = transliterated
		}
		var candidates []candidate
		for rows.Next() {
			var rawJSON, matchedRow []byte
			if err := rows.Scan(&rawJSON, &matchedRow); err != nil {
				continue
			}
			name := ""
			score := 0
			// 1. raw_json.name_cyr (most reliable — from parsed passport).
			if len(rawJSON) > 0 {
				var raw map[string]any
				if err := json.Unmarshal(rawJSON, &raw); err == nil {
					if s, ok := raw["name_cyr"].(string); ok && s != "" {
						name = s
						score = 2
					}
				}
			}
			// 2. matched_sheet_row Cyrillic column.
			if name == "" && len(matchedRow) > 0 {
				var m map[string]string
				if err := json.Unmarshal(matchedRow, &m); err == nil {
					for _, key := range []string{"ФИО (кириллицей)", "ФИО кириллицей", "ФИО"} {
						if v := m[key]; v != "" {
							name = v
							score = 1
							break
						}
					}
					// 2b. Sheet "latin" column may actually contain Cyrillic text.
					if name == "" {
						for _, key := range []string{"ФИО (латиницей, как в загранпаспорте)", "ФИО латиницей", "ФИО (латиницей)"} {
							if v := m[key]; v != "" {
								if isCyrillic(v) {
									name = v
									score = 1
								} else {
									name = translitLatToCyr(v)
									score = 0
								}
								break
							}
						}
					}
				}
			}
			// 3. raw_json.name_lat — last resort, transliterated.
			if name == "" && len(rawJSON) > 0 {
				var raw map[string]any
				if err := json.Unmarshal(rawJSON, &raw); err == nil {
					if s, ok := raw["name_lat"].(string); ok && s != "" {
						name = translitLatToCyr(s)
						score = 0
					}
				}
			}
			if name == "" {
				continue
			}
			candidates = append(candidates, candidate{name: name, score: score})
		}
		rows.Close()

		// Fuzzy dedupe preserving creation order. When a duplicate is found (edit
		// distance ≤ 2 on normalized key), keep the version with the higher score
		// but leave its position where the first occurrence was.
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
			writeError(w, http.StatusBadRequest, "no confirmed tourists found in this group")
			return
		}

		// Patch the template JSON: override inna_doc.applicants_ru and vc_request.
		var doc map[string]any
		if err := json.Unmarshal(templateJSON, &doc); err != nil {
			slog.Error("unmarshal template pass2", "err", err)
			writeError(w, http.StatusInternalServerError, "corrupt template JSON")
			return
		}
		if inna, ok := doc["inna_doc"].(map[string]any); ok {
			inna["applicants_ru"] = applicantsRu
			doc["inna_doc"] = inna
		} else {
			doc["inna_doc"] = map[string]any{"applicants_ru": applicantsRu}
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

		zipPath, err := docgen.GenerateFinal(r.Context(), pythonScript, uploadsDir, groupID, mergedJSON)
		if err != nil {
			slog.Error("docgen final", "err", err)
			writeError(w, http.StatusInternalServerError, "final document generation failed: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"zip_path": zipPath, "count": len(applicantsRu)})
	}
}

// GetDocuments handles GET /api/groups/:id/documents
func GetDocuments(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		rows, err := db.Query(r.Context(),
			`SELECT id, group_id, pass2_json, zip_path, generated_at, created_at
			   FROM documents WHERE group_id = $1 ORDER BY created_at DESC`, groupID)
		if err != nil {
			slog.Error("fetch documents", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		type Doc struct {
			ID          string          `json:"id"`
			GroupID     string          `json:"group_id"`
			Pass2JSON   json.RawMessage `json:"pass2_json"`
			ZipPath     string          `json:"zip_path"`
			GeneratedAt *time.Time      `json:"generated_at"`
			CreatedAt   time.Time       `json:"created_at"`
		}

		var docs []Doc
		for rows.Next() {
			var d Doc
			var pass2JSON []byte
			if err := rows.Scan(&d.ID, &d.GroupID, &pass2JSON, &d.ZipPath, &d.GeneratedAt, &d.CreatedAt); err != nil {
				slog.Error("scan document", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			if pass2JSON != nil {
				d.Pass2JSON = json.RawMessage(pass2JSON)
			}
			docs = append(docs, d)
		}
		if docs == nil {
			docs = []Doc{}
		}
		writeJSON(w, http.StatusOK, docs)
	}
}

// FinalStatus handles GET /api/groups/:id/final/status
// Returns whether the final.zip exists on disk and when it was generated.
func FinalStatus(uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")
		zipPath := filepath.Join(uploadsDir, groupID, "final.zip")
		resp := map[string]any{"has_zip": false}
		if info, err := os.Stat(zipPath); err == nil && !info.IsDir() {
			resp["has_zip"] = true
			resp["generated_at"] = info.ModTime()
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// DownloadFinalZIP handles GET /api/groups/:id/download/final
func DownloadFinalZIP(uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")
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
func DownloadZIP(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		var zipPath string
		err := db.QueryRow(r.Context(),
			`SELECT zip_path FROM documents WHERE group_id = $1 ORDER BY generated_at DESC LIMIT 1`,
			groupID,
		).Scan(&zipPath)
		if err != nil {
			writeError(w, http.StatusNotFound, "no documents found for this group")
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(zipPath)+`"`)
		http.ServeFile(w, r, zipPath)
	}
}
