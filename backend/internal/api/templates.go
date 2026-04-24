package api

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fujitravel-admin/backend/internal/middleware"
)

// Max .docx size for a template upload. Real Word templates are tiny
// (10–100 KB); 5 MB is a generous safety ceiling.
const maxTemplateUploadSize = 5 << 20 // 5 MB

// doverenostTemplatePath returns the on-disk path of the org's custom
// доверенность template, whether it exists yet or not.
func doverenostTemplatePath(uploadsDir, orgID string) string {
	return filepath.Join(uploadsDir, orgID, "templates", "doverenost.docx")
}

// GetDoverenostTemplateStatus handles GET /api/templates/doverenost
// Returns {custom: bool, uploaded_at?: string, size?: int}.
func GetDoverenostTemplateStatus(uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		path := doverenostTemplatePath(uploadsDir, orgID)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSON(w, http.StatusOK, map[string]any{"custom": false})
				return
			}
			slog.Error("stat doverenost template", "err", err)
			writeError(w, http.StatusInternalServerError, "fs error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"custom":      true,
			"uploaded_at": info.ModTime().UTC().Format(time.RFC3339),
			"size":        info.Size(),
		})
	}
}

// UploadDoverenostTemplate handles POST /api/templates/doverenost
// Multipart form with a single "file" field containing the .docx.
func UploadDoverenostTemplate(uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())

		if err := r.ParseMultipartForm(maxTemplateUploadSize); err != nil {
			writeError(w, http.StatusBadRequest, "failed to parse multipart form (max 5 MB)")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing 'file' field")
			return
		}
		defer file.Close()

		if !strings.HasSuffix(strings.ToLower(header.Filename), ".docx") {
			writeError(w, http.StatusBadRequest, "only .docx files are accepted")
			return
		}
		if header.Size > maxTemplateUploadSize {
			writeError(w, http.StatusBadRequest, "file exceeds 5 MB limit")
			return
		}

		destPath := doverenostTemplatePath(uploadsDir, orgID)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			slog.Error("mkdir templates", "err", err)
			writeError(w, http.StatusInternalServerError, "fs error")
			return
		}

		// Write atomically: stage under a sibling .tmp path and rename.
		tmpPath := destPath + ".tmp"
		out, err := os.Create(tmpPath)
		if err != nil {
			slog.Error("create tmp template", "err", err)
			writeError(w, http.StatusInternalServerError, "fs error")
			return
		}
		if _, err := io.Copy(out, file); err != nil {
			out.Close()
			os.Remove(tmpPath) //nolint:errcheck
			slog.Error("copy template body", "err", err)
			writeError(w, http.StatusInternalServerError, "fs error")
			return
		}
		if err := out.Close(); err != nil {
			os.Remove(tmpPath) //nolint:errcheck
			slog.Error("close tmp template", "err", err)
			writeError(w, http.StatusInternalServerError, "fs error")
			return
		}
		if err := os.Rename(tmpPath, destPath); err != nil {
			os.Remove(tmpPath) //nolint:errcheck
			slog.Error("rename template", "err", err)
			writeError(w, http.StatusInternalServerError, "fs error")
			return
		}

		info, _ := os.Stat(destPath)
		writeJSON(w, http.StatusOK, map[string]any{
			"custom":      true,
			"uploaded_at": info.ModTime().UTC().Format(time.RFC3339),
			"size":        info.Size(),
		})
	}
}

// DeleteDoverenostTemplate handles DELETE /api/templates/doverenost.
// Reverts the org back to the bundled default template.
func DeleteDoverenostTemplate(uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		path := doverenostTemplatePath(uploadsDir, orgID)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			slog.Error("delete doverenost template", "err", err)
			writeError(w, http.StatusInternalServerError, "fs error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DownloadDoverenostTemplate handles GET /api/templates/doverenost/download.
// Streams the org's custom .docx if present; 404 if using bundled default.
func DownloadDoverenostTemplate(uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		path := doverenostTemplatePath(uploadsDir, orgID)
		if _, err := os.Stat(path); err != nil {
			writeError(w, http.StatusNotFound, "no custom template uploaded")
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		w.Header().Set("Content-Disposition", `attachment; filename="doverenost_template.docx"`)
		http.ServeFile(w, r, path)
	}
}
