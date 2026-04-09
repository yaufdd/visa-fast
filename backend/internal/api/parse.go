package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/ai"
	"fujitravel-admin/backend/internal/storage"
)

// loadUploadsAsFileInputs reads the given uploads and, for each row missing an
// anthropic_file_id, tries to upload it to the Anthropic Files API now and cache
// the returned ID. Falls back to inline bytes only if the upload still fails.
func loadUploadsAsFileInputs(ctx context.Context, db *pgxpool.Pool, apiKey string, uploads []struct{ Path, FileID string }) ([]ai.FileInput, error) {
	var inputs []ai.FileInput
	for _, u := range uploads {
		if u.FileID != "" {
			inputs = append(inputs, ai.FileInput{AnthropicFileID: u.FileID, Name: filepath.Base(u.Path)})
			continue
		}
		data, err := storage.ReadFile(u.Path)
		if err != nil {
			return nil, fmt.Errorf("read upload %s: %w", u.Path, err)
		}
		// Lazy upload to Anthropic Files API.
		if fid, err := ai.UploadFileToAnthropic(apiKey, filepath.Base(u.Path), data); err == nil && fid != "" {
			_, _ = db.Exec(ctx,
				`UPDATE uploads SET anthropic_file_id = $1 WHERE file_path = $2`,
				fid, u.Path)
			inputs = append(inputs, ai.FileInput{AnthropicFileID: fid, Name: filepath.Base(u.Path)})
		} else {
			if err != nil {
				slog.Warn("lazy anthropic upload failed, using inline", "path", u.Path, "err", err)
			}
			inputs = append(inputs, ai.FileInput{Name: filepath.Base(u.Path), Data: data})
		}
	}
	return inputs, nil
}

// normalizeForMatch lowercases, strips punctuation/whitespace, and returns a
// compact key suitable for equality or Levenshtein comparison. Works for both
// Cyrillic and Latin names.
func normalizeForMatch(s string) string {
	var out []rune
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= 'а' && r <= 'я') || r == 'ё' {
			out = append(out, r)
		} else if r == ' ' && len(out) > 0 && out[len(out)-1] != ' ' {
			out = append(out, ' ')
		}
	}
	return strings.TrimSpace(string(out))
}

// escapeLikePattern escapes LIKE metacharacters (%, _) in user input so that
// a voucher hotel name like "A_1" isn't treated as a wildcard match.
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// convertDate converts DD.MM.YYYY → YYYY-MM-DD for PostgreSQL date fields.
func convertDate(s string) string {
	if len(s) == 10 && s[2] == '.' && s[5] == '.' {
		return s[6:10] + "-" + s[3:5] + "-" + s[0:2]
	}
	return s
}

// ParseTourist handles POST /api/tourists/:id/parse
// Reads uploads for a specific tourist, runs Pass 1, saves raw_json.
func ParseTourist(db *pgxpool.Pool, apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		touristID := chi.URLParam(r, "id")

		rows, err := db.Query(r.Context(),
			`SELECT file_path, COALESCE(anthropic_file_id,'') FROM uploads WHERE tourist_id = $1 ORDER BY created_at`, touristID)
		if err != nil {
			slog.Error("fetch tourist uploads", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		var uploadsList []struct{ Path, FileID string }
		for rows.Next() {
			var path, fileID string
			if err := rows.Scan(&path, &fileID); err != nil {
				continue
			}
			uploadsList = append(uploadsList, struct{ Path, FileID string }{path, fileID})
		}
		rows.Close()

		inputs, err := loadUploadsAsFileInputs(r.Context(), db, apiKey, uploadsList)
		if err != nil {
			slog.Error("load uploads as file inputs", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to prepare files: "+err.Error())
			return
		}

		if len(inputs) == 0 {
			writeError(w, http.StatusBadRequest, "no uploads found for this tourist")
			return
		}

		results, err := ai.ParseDocuments(r.Context(), apiKey, inputs)
		if err != nil {
			slog.Error("ai pass1 tourist", "err", err)
			writeError(w, http.StatusInternalServerError, "AI parsing failed: "+err.Error())
			return
		}

		// Use first result (tourist-specific upload should yield one person).
		if len(results) == 0 {
			writeError(w, http.StatusInternalServerError, "AI returned no results")
			return
		}
		result := results[0]
		rawJSON, _ := json.Marshal(result)

		if _, err := db.Exec(r.Context(),
			`UPDATE tourists SET raw_json = $1, updated_at = now() WHERE id = $2`,
			rawJSON, touristID); err != nil {
			slog.Error("update tourist raw_json", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}

		// Auto-populate group_hotels from voucher data if not already set.
		if len(result.HotelsFromVouchers) > 0 {
			var existing int
			_ = db.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM group_hotels WHERE group_id = (SELECT group_id FROM tourists WHERE id = $1)`,
				touristID).Scan(&existing)

			if existing == 0 {
				var groupID string
				_ = db.QueryRow(r.Context(), `SELECT group_id FROM tourists WHERE id = $1`, touristID).Scan(&groupID)

				for i, vh := range result.HotelsFromVouchers {
					// Find hotel by fuzzy name match in DB.
					var hotelID string
					err := db.QueryRow(r.Context(),
						`SELECT id FROM hotels WHERE lower(name_en) ILIKE $1 LIMIT 1`,
						"%"+vh.Name+"%",
					).Scan(&hotelID)
					if err != nil {
						// Try substring match the other way.
						_ = db.QueryRow(r.Context(),
							`SELECT id FROM hotels WHERE $1 ILIKE '%' || lower(name_en) || '%' LIMIT 1`,
							strings.ToLower(vh.Name),
						).Scan(&hotelID)
					}
					if hotelID == "" {
						// Hotel not in DB — create it automatically from voucher data.
						err = db.QueryRow(r.Context(),
							`INSERT INTO hotels (name_en, city) VALUES ($1, '') RETURNING id`,
							vh.Name,
						).Scan(&hotelID)
						if err != nil {
							slog.Warn("auto-create hotel from voucher failed", "name", vh.Name, "err", err)
							continue
						}
						slog.Info("auto-created hotel from voucher", "name", vh.Name, "id", hotelID)
					}
					// Convert DD.MM.YYYY → YYYY-MM-DD for PostgreSQL.
					checkIn := convertDate(vh.CheckIn)
					checkOut := convertDate(vh.CheckOut)
					if _, err := db.Exec(r.Context(),
						`INSERT INTO group_hotels (group_id, hotel_id, check_in, check_out, sort_order)
						 VALUES ($1, $2, $3::date, $4::date, $5)
						 ON CONFLICT DO NOTHING`,
						groupID, hotelID, checkIn, checkOut, i,
					); err != nil {
						slog.Warn("insert group_hotel from voucher", "err", err)
					}
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{"result": result})
	}
}

// ParseGroup handles POST /api/groups/:id/parse
// Reads all uploads for the group, sends them to Claude Pass 1, saves results as tourists.
// If a SheetsSearcher is provided, automatically tries to match the tourist against Google Sheets.
func ParseGroup(db *pgxpool.Pool, apiKey string, sheets ...SheetsSearcher) http.HandlerFunc {
	var sheetsClient SheetsSearcher
	if len(sheets) > 0 {
		sheetsClient = sheets[0]
	}
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := chi.URLParam(r, "id")

		// Verify group exists.
		var groupExists bool
		if err := db.QueryRow(r.Context(), `SELECT true FROM groups WHERE id = $1`, groupID).Scan(&groupExists); err != nil {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}

		// Fetch all uploads for this group.
		rows, err := db.Query(r.Context(),
			`SELECT file_path, COALESCE(anthropic_file_id,'') FROM uploads WHERE group_id = $1 ORDER BY created_at`,
			groupID)
		if err != nil {
			slog.Error("fetch uploads", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		var uploadsList []struct{ Path, FileID string }
		for rows.Next() {
			var path, fileID string
			if err := rows.Scan(&path, &fileID); err != nil {
				slog.Error("scan upload row", "err", err)
				writeError(w, http.StatusInternalServerError, "scan error")
				return
			}
			uploadsList = append(uploadsList, struct{ Path, FileID string }{path, fileID})
		}
		rows.Close()

		inputs, err := loadUploadsAsFileInputs(r.Context(), db, apiKey, uploadsList)
		if err != nil {
			slog.Error("load uploads as file inputs", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to prepare files: "+err.Error())
			return
		}

		if len(inputs) == 0 {
			writeError(w, http.StatusBadRequest, "no uploads found for this group")
			return
		}

		// Optional manager notes passed as query param.
		notes := r.URL.Query().Get("notes")

		result, err := ai.ParseDocuments(r.Context(), apiKey, inputs, notes)
		if err != nil {
			slog.Error("ai pass1", "err", err)
			writeError(w, http.StatusInternalServerError, "AI parsing failed: "+err.Error())
			return
		}

		// Load existing tourists for this group (pre-selected from Sheets).
		// Try all known name-column variants from the Google Sheet.
		existingRows, _ := db.Query(r.Context(),
			`SELECT id,
			        COALESCE(
			            NULLIF(matched_sheet_row->>'ФИО латиницей', ''),
			            NULLIF(matched_sheet_row->>'ФИО (латиницей, как в загранпаспорте)', ''),
			            NULLIF(matched_sheet_row->>'ФИО (латиницей)', ''),
			            NULLIF(raw_json->>'name_lat', ''),
			            ''
			        )
			   FROM tourists WHERE group_id = $1`, groupID)
		existingByName := map[string]string{} // name_lat → tourist_id
		if existingRows != nil {
			for existingRows.Next() {
				var tid, name string
				if err := existingRows.Scan(&tid, &name); err == nil && name != "" {
					upper := strings.ToUpper(strings.TrimSpace(name))
					existingByName[upper] = tid
					// Also index the reversed word order (FIRSTNAME LASTNAME ↔ LASTNAME FIRSTNAME).
					parts := strings.Fields(upper)
					if len(parts) == 2 {
						existingByName[parts[1]+" "+parts[0]] = tid
					}
				}
			}
			existingRows.Close()
		}

		type touristOut struct {
			TouristID      string         `json:"tourist_id"`
			Result         ai.Pass1Result `json:"result"`
			MatchConfirmed bool           `json:"match_confirmed"`
		}
		var created []touristOut

		for _, t := range result {
			rawJSON, err := json.Marshal(t)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "marshal result error")
				return
			}

			// Try to match to an existing tourist by name_lat (handles both word orders).
			existingID := existingByName[strings.ToUpper(strings.TrimSpace(t.NameLat))]

			var touristID string
			if existingID != "" {
				// Update existing tourist's raw_json.
				if _, err := db.Exec(r.Context(),
					`UPDATE tourists SET raw_json = $1, updated_at = now() WHERE id = $2`,
					rawJSON, existingID); err != nil {
					slog.Error("update tourist raw_json", "err", err)
					writeError(w, http.StatusInternalServerError, "database error")
					return
				}
				touristID = existingID
				slog.Info("updated existing tourist from parse", "id", touristID, "name", t.NameLat)
			} else {
				// No existing match — create new tourist record.
				var matchedRowJSON []byte
				matchConfirmed := false
				if sheetsClient != nil {
					query := t.NameLat
					if query == "" {
						query = t.NameCyr
					}
					matches, merr := sheetsClient.SearchByName(r.Context(), query, 1)
					if merr != nil {
						slog.Warn("auto-match sheets search failed", "err", merr)
					} else if len(matches) > 0 && matches[0].Score >= 0.5 {
						matchedRowJSON, _ = json.Marshal(matches[0].Row)
						matchConfirmed = true
					}
				}
				err = db.QueryRow(r.Context(),
					`INSERT INTO tourists (group_id, raw_json, matched_sheet_row, match_confirmed)
					 VALUES ($1, $2, $3, $4) RETURNING id`,
					groupID, rawJSON, matchedRowJSON, matchConfirmed,
				).Scan(&touristID)
				if err != nil {
					slog.Error("insert tourist", "err", err)
					writeError(w, http.StatusInternalServerError, "database error")
					return
				}
			}
			created = append(created, touristOut{TouristID: touristID, Result: t, MatchConfirmed: existingID != ""})
		}

		// Auto-populate group_hotels from voucher data (only if no hotels configured yet).
		var existingHotels int
		_ = db.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM group_hotels WHERE group_id = $1`, groupID).Scan(&existingHotels)

		if existingHotels == 0 {
			// Collect hotels from first tourist that has voucher data.
			var vouchers []ai.VoucherHotel
			for _, t := range result {
				if len(t.HotelsFromVouchers) > 0 {
					vouchers = t.HotelsFromVouchers
					break
				}
			}
			for i, vh := range vouchers {
				var hotelID string
				err := db.QueryRow(r.Context(),
					`SELECT id FROM hotels WHERE lower(name_en) ILIKE $1 LIMIT 1`,
					"%"+strings.ToLower(vh.Name)+"%",
				).Scan(&hotelID)
				if err != nil {
					_ = db.QueryRow(r.Context(),
						`SELECT id FROM hotels WHERE $1 ILIKE '%' || lower(name_en) || '%' LIMIT 1`,
						strings.ToLower(vh.Name),
					).Scan(&hotelID)
				}
				if hotelID == "" {
					err = db.QueryRow(r.Context(),
						`INSERT INTO hotels (name_en, city) VALUES ($1, '') RETURNING id`,
						vh.Name,
					).Scan(&hotelID)
					if err != nil {
						slog.Warn("auto-create hotel from voucher failed", "name", vh.Name, "err", err)
						continue
					}
					slog.Info("auto-created hotel from voucher", "name", vh.Name, "id", hotelID)
				}
				checkIn := convertDate(vh.CheckIn)
				checkOut := convertDate(vh.CheckOut)
				if _, err := db.Exec(r.Context(),
					`INSERT INTO group_hotels (group_id, hotel_id, check_in, check_out, sort_order)
					 VALUES ($1, $2, $3::date, $4::date, $5) ON CONFLICT DO NOTHING`,
					groupID, hotelID, checkIn, checkOut, i,
				); err != nil {
					slog.Warn("insert group_hotel from voucher", "err", err)
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"tourists": created,
			"count":    len(created),
		})
	}
}

// ParseSubgroup handles POST /api/subgroups/:id/parse
// Reads uploads for a subgroup, runs Pass 1, updates tourists in that subgroup.
func ParseSubgroup(db *pgxpool.Pool, apiKey string, sheets ...SheetsSearcher) http.HandlerFunc {
	var sheetsClient SheetsSearcher
	if len(sheets) > 0 {
		sheetsClient = sheets[0]
	}
	return func(w http.ResponseWriter, r *http.Request) {
		subgroupID := chi.URLParam(r, "id")

		// Get group_id for this subgroup.
		var groupID string
		if err := db.QueryRow(r.Context(),
			`SELECT group_id FROM subgroups WHERE id = $1`, subgroupID).Scan(&groupID); err != nil {
			writeError(w, http.StatusNotFound, "subgroup not found")
			return
		}

		// Fetch uploads for this subgroup.
		rows, err := db.Query(r.Context(),
			`SELECT file_path, COALESCE(anthropic_file_id,'') FROM uploads WHERE subgroup_id = $1 ORDER BY created_at`,
			subgroupID)
		if err != nil {
			slog.Error("fetch subgroup uploads", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		var uploadsList []struct{ Path, FileID string }
		for rows.Next() {
			var path, fileID string
			if err := rows.Scan(&path, &fileID); err != nil {
				continue
			}
			uploadsList = append(uploadsList, struct{ Path, FileID string }{path, fileID})
		}
		rows.Close()

		inputs, err := loadUploadsAsFileInputs(r.Context(), db, apiKey, uploadsList)
		if err != nil {
			slog.Error("load uploads as file inputs", "err", err)
			writeError(w, http.StatusInternalServerError, "failed to prepare files: "+err.Error())
			return
		}

		if len(inputs) == 0 {
			writeError(w, http.StatusBadRequest, "no uploads found for this subgroup")
			return
		}

		notes := r.URL.Query().Get("notes")

		result, err := ai.ParseDocuments(r.Context(), apiKey, inputs, notes)
		if err != nil {
			slog.Error("ai pass1 subgroup", "err", err)
			writeError(w, http.StatusInternalServerError, "AI parsing failed: "+err.Error())
			return
		}

		// Load existing tourists in this subgroup with ALL name candidates we can
		// derive from sheet row + raw_json. We'll match the parsed tourist against
		// any of them, allowing for small typos via Levenshtein distance.
		type existingEntry struct {
			id    string
			names []string
		}
		var existing []existingEntry
		existingExact := map[string]string{} // normalized → tourist_id (fast path)

		eRows, eErr := db.Query(r.Context(),
			`SELECT id,
			        COALESCE(NULLIF(matched_sheet_row->>'ФИО латиницей', ''), ''),
			        COALESCE(NULLIF(matched_sheet_row->>'ФИО (латиницей, как в загранпаспорте)', ''), ''),
			        COALESCE(NULLIF(matched_sheet_row->>'ФИО (латиницей)', ''), ''),
			        COALESCE(NULLIF(matched_sheet_row->>'ФИО (кириллицей)', ''), ''),
			        COALESCE(NULLIF(matched_sheet_row->>'ФИО', ''), ''),
			        COALESCE(NULLIF(raw_json->>'name_lat', ''), ''),
			        COALESCE(NULLIF(raw_json->>'name_cyr', ''), '')
			   FROM tourists WHERE subgroup_id = $1`, subgroupID)
		if eErr == nil {
			for eRows.Next() {
				var tid, s1, s2, s3, s4, s5, rl, rc string
				if err := eRows.Scan(&tid, &s1, &s2, &s3, &s4, &s5, &rl, &rc); err != nil {
					continue
				}
				var names []string
				for _, n := range []string{s1, s2, s3, s4, s5, rl, rc} {
					if n == "" {
						continue
					}
					names = append(names, n)
					key := normalizeForMatch(n)
					if key != "" {
						existingExact[key] = tid
						// Also index the reversed word order (FIRSTNAME LASTNAME ↔ LASTNAME FIRSTNAME).
						parts := strings.Fields(key)
						if len(parts) == 2 {
							existingExact[parts[1]+" "+parts[0]] = tid
						}
					}
				}
				if len(names) > 0 {
					existing = append(existing, existingEntry{id: tid, names: names})
				}
			}
			eRows.Close()
		}

		type touristOut struct {
			TouristID      string         `json:"tourist_id"`
			Result         ai.Pass1Result `json:"result"`
			MatchConfirmed bool           `json:"match_confirmed"`
		}
		var created []touristOut

		for _, t := range result {
			rawJSON, err := json.Marshal(t)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "marshal result error")
				return
			}

			// Try to match parsed tourist against an existing one by any Latin or
			// Cyrillic name candidate, exact first then fuzzy (Levenshtein ≤ 2).
			// This catches sheet typos like "Щербаоков" vs "Щербаков".
			existingID := ""
			parsedKeys := []string{}
			for _, candidate := range []string{t.NameLat, t.NameCyr} {
				if candidate != "" {
					k := normalizeForMatch(candidate)
					parsedKeys = append(parsedKeys, k)
					if id, ok := existingExact[k]; ok && existingID == "" {
						existingID = id
					}
				}
			}
			if existingID == "" {
			fuzzy:
				for _, e := range existing {
					for _, name := range e.names {
						ek := normalizeForMatch(name)
						if ek == "" {
							continue
						}
						for _, pk := range parsedKeys {
							if editDistance(ek, pk) <= 2 {
								existingID = e.id
								break fuzzy
							}
						}
					}
				}
			}

			var touristID string
			if existingID != "" {
				if _, err := db.Exec(r.Context(),
					`UPDATE tourists SET raw_json = $1, updated_at = now() WHERE id = $2`,
					rawJSON, existingID); err != nil {
					slog.Error("update tourist raw_json", "err", err)
					writeError(w, http.StatusInternalServerError, "database error")
					return
				}
				touristID = existingID
			} else {
				var matchedRowJSON []byte
				matchConfirmed := false
				if sheetsClient != nil {
					query := t.NameLat
					if query == "" {
						query = t.NameCyr
					}
					matches, merr := sheetsClient.SearchByName(r.Context(), query, 1)
					if merr != nil {
						slog.Warn("auto-match sheets search failed", "err", merr)
					} else if len(matches) > 0 && matches[0].Score >= 0.5 {
						matchedRowJSON, _ = json.Marshal(matches[0].Row)
						matchConfirmed = true
					}
				}
				err = db.QueryRow(r.Context(),
					`INSERT INTO tourists (group_id, subgroup_id, raw_json, matched_sheet_row, match_confirmed)
					 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
					groupID, subgroupID, rawJSON, matchedRowJSON, matchConfirmed,
				).Scan(&touristID)
				if err != nil {
					slog.Error("insert tourist", "err", err)
					writeError(w, http.StatusInternalServerError, "database error")
					return
				}
			}
			created = append(created, touristOut{TouristID: touristID, Result: t, MatchConfirmed: existingID != ""})
		}

		// Auto-populate hotels for THIS subgroup from vouchers (only if subgroup has none yet).
		var existingHotels int
		_ = db.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM group_hotels WHERE subgroup_id = $1`, subgroupID).Scan(&existingHotels)
		if existingHotels == 0 {
			var vouchers []ai.VoucherHotel
			for _, t := range result {
				if len(t.HotelsFromVouchers) > 0 {
					vouchers = t.HotelsFromVouchers
					break
				}
			}
			for i, vh := range vouchers {
				var hotelID string
				escapedLower := escapeLikePattern(strings.ToLower(vh.Name))
				err := db.QueryRow(r.Context(),
					`SELECT id FROM hotels WHERE lower(name_en) ILIKE $1 ESCAPE '\' LIMIT 1`,
					"%"+escapedLower+"%").Scan(&hotelID)
				if err != nil {
					_ = db.QueryRow(r.Context(),
						`SELECT id FROM hotels WHERE $1 ILIKE '%' || lower(name_en) || '%' LIMIT 1`,
						strings.ToLower(vh.Name)).Scan(&hotelID)
				}
				if hotelID == "" {
					err = db.QueryRow(r.Context(),
						`INSERT INTO hotels (name_en, city) VALUES ($1, '') RETURNING id`,
						vh.Name).Scan(&hotelID)
					if err != nil {
						slog.Warn("auto-create hotel failed", "name", vh.Name, "err", err)
						continue
					}
				}
				checkIn := convertDate(vh.CheckIn)
				checkOut := convertDate(vh.CheckOut)
				if _, err := db.Exec(r.Context(),
					`INSERT INTO group_hotels (group_id, subgroup_id, hotel_id, check_in, check_out, sort_order)
					 VALUES ($1, $2, $3, $4::date, $5::date, $6) ON CONFLICT DO NOTHING`,
					groupID, subgroupID, hotelID, checkIn, checkOut, i); err != nil {
					slog.Warn("insert subgroup hotel from voucher", "err", err)
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"tourists": created,
			"count":    len(created),
		})
	}
}
