package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/ai"
	"fujitravel-admin/backend/internal/api"
	appmw "fujitravel-admin/backend/internal/middleware"
)

// NewRouter builds the full chi router with public + protected groups.
// Used by both main.go and integration tests.
//
// translator is the Yandex-backed Translator wired in cmd/server/main.go.
// ocrClient is the Yandex Vision OCR seam used by the scan parsers
// (ticket / voucher / passport). Tests that don't exercise the /generate
// or /uploads-parse paths can pass nil — the router simply forwards both
// to the handlers, which only dereference them inside the relevant
// codepaths.
func NewRouter(pool *pgxpool.Pool, translator ai.Translator, ocrClient ai.OCRRecognizer, uploadsDir, pythonScript string) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	// CORS — simple permissive policy for local dev.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if req.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	// Rate limiters.
	registerRL := appmw.NewRateLimiter(5, 15*time.Minute)
	loginRL := appmw.NewRateLimiter(10, 15*time.Minute)
	// publicRL is the strict bucket for row-creation endpoints on the
	// public form (anti-spam against bots probing the slug pool).
	publicRL := appmw.NewRateLimiter(3, 10*time.Minute)
	// publicFileRL covers the file CRUD path; a real tourist uploads
	// four scans (passport_internal, passport_foreign, ticket, voucher)
	// and may replace one or two of them, which would trip publicRL.
	publicFileRL := appmw.NewRateLimiter(30, 10*time.Minute)

	r.Route("/api", func(r chi.Router) {
		// Health
		r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
			if err := pool.Ping(req.Context()); err != nil {
				w.WriteHeader(503)
				return
			}
			w.WriteHeader(200)
		})

		// Public — auth
		r.With(registerRL.Middleware()).Post("/auth/register", api.Register(pool))
		r.With(loginRL.Middleware()).Post("/auth/login", api.Login(pool))

		// Public — slug form
		r.Get("/public/org/{slug}", api.PublicOrg(pool))
		r.With(publicRL.Middleware()).Post("/public/submissions/{slug}", api.PublicSubmit(pool))

		// Public — draft submission + scan attachments. /start uses the
		// strict publicRL (it creates a row); /files* use the looser
		// publicFileRL so legitimate scan uploads do not get 429'd
		// mid-form.
		r.With(publicRL.Middleware()).Post("/public/submissions/{slug}/start", api.PublicSubmissionStart(pool))
		r.With(publicFileRL.Middleware()).Post("/public/submissions/{slug}/files/{type}", api.PublicUploadSubmissionFile(pool, uploadsDir))
		r.With(publicFileRL.Middleware()).Get("/public/submissions/{slug}/files", api.PublicListSubmissionFiles(pool))
		r.With(publicFileRL.Middleware()).Delete("/public/submissions/{slug}/files/{id}", api.PublicDeleteSubmissionFile(pool, uploadsDir))
		r.With(publicFileRL.Middleware()).Post("/public/submissions/{slug}/parse-passport", api.PublicParsePassport(pool, ocrClient, translator))

		// Public — consent text
		r.Get("/consent/text", api.GetConsentText())

		// Protected — everything else
		r.Group(func(r chi.Router) {
			r.Use(appmw.RequireAuth(pool))

			r.Post("/auth/logout", api.Logout(pool))
			r.Get("/auth/me", api.Me(pool))

			// Hotels
			r.Get("/hotels", api.ListHotels(pool))
			r.Post("/hotels", api.CreateHotel(pool))
			r.Get("/hotels/{id}", api.GetHotel(pool))
			r.Put("/hotels/{id}", api.UpdateHotel(pool))

			// Groups
			r.Get("/groups", api.ListGroups(pool))
			r.Post("/groups", api.CreateGroup(pool))
			r.Get("/groups/{id}", api.GetGroup(pool))
			r.Delete("/groups/{id}", api.DeleteGroup(pool))
			r.Put("/groups/{id}/name", api.UpdateGroupName(pool))
			r.Put("/groups/{id}/status", api.UpdateGroupStatus(pool))
			r.Put("/groups/{id}/notes", api.UpdateGroupNotes(pool))
			r.Put("/groups/{id}/programme_notes", api.UpdateGroupProgrammeNotes(pool))

			// Subgroups
			r.Get("/groups/{id}/subgroups", api.ListSubgroups(pool, uploadsDir))
			r.Post("/groups/{id}/subgroups", api.CreateSubgroup(pool))
			r.Put("/subgroups/{id}", api.UpdateSubgroup(pool))
			r.Put("/subgroups/{id}/programme_notes", api.UpdateSubgroupProgrammeNotes(pool))
			r.Delete("/subgroups/{id}", api.DeleteSubgroup(pool))
			r.Put("/tourists/{id}/subgroup", api.AssignTouristSubgroup(pool))
			r.Get("/subgroups/{id}/hotels", api.ListSubgroupHotels(pool))
			r.Post("/subgroups/{id}/hotels", api.UpsertSubgroupHotels(pool))
			r.Post("/subgroups/{id}/generate", api.GenerateSubgroupDocuments(pool, translator, uploadsDir, pythonScript))
			r.Get("/subgroups/{id}/download", api.DownloadSubgroupZIP(pool, uploadsDir))

			// Tourists
			r.Get("/groups/{id}/tourists", api.ListTourists(pool))
			r.Delete("/tourists/{id}", api.DeleteTourist(pool))

			// Per-tourist uploads
			r.Get("/tourists/{id}/uploads", api.ListTouristUploads(pool))
			r.Post("/tourists/{id}/uploads", api.UploadTouristFile(pool, uploadsDir))
			r.Post("/tourists/{id}/uploads/{uploadId}/parse", api.ParseTouristUpload(pool, ocrClient, translator, uploadsDir))
			r.Delete("/tourists/{id}/uploads/{uploadId}", api.DeleteTouristUpload(pool))

			// Group hotels
			r.Get("/groups/{id}/hotels", api.ListGroupHotels(pool))
			r.Post("/groups/{id}/hotels", api.UpsertGroupHotels(pool))

			// Document generation (AI Pass 2 + Python)
			r.Post("/groups/{id}/generate", api.GenerateDocuments(pool, translator, uploadsDir, pythonScript))
			r.Post("/groups/{id}/finalize", api.FinalizeGroup(pool, uploadsDir, pythonScript))
			r.Get("/groups/{id}/documents", api.GetDocuments(pool))
			r.Get("/groups/{id}/download", api.DownloadZIP(pool))
			r.Get("/groups/{id}/download/final", api.DownloadFinalZIP(pool, uploadsDir))
			r.Get("/groups/{id}/final/status", api.FinalStatus(pool, uploadsDir))

			// AI audit log — every provider call made on behalf of this group.
			r.Get("/groups/{id}/ai_logs", api.ListGroupAILogs(pool))

			// Submission-file counts per tourist in a group — single
			// round-trip used by GroupDetailPage to render the 📎 N
			// badge without an N+1 burst of /files calls.
			r.Get("/groups/{id}/tourist_file_counts", api.GroupTouristFileCounts(pool))

			// Submissions (form-based workflow)
			r.Post("/submissions", api.CreateSubmissionByManager(pool))
			r.Post("/submissions/draft", api.CreateDraftSubmissionAsManager(pool))
			r.Get("/submissions", api.ListSubmissions(pool))
			r.Get("/submissions/{id}", api.GetSubmission(pool))
			r.Put("/submissions/{id}", api.UpdateSubmission(pool))
			r.Delete("/submissions/{id}", api.ArchiveSubmission(pool))
			r.Delete("/submissions/{id}/erase", api.EraseSubmission(pool))
			r.Post("/submissions/{id}/attach", api.AttachSubmission(pool))

			// Manager view of files a tourist attached via the public form.
			r.Get("/submissions/{id}/files", api.ListSubmissionFiles(pool))
			r.Get("/submissions/{id}/files/{file_id}/download", api.DownloadSubmissionFile(pool))
			r.Delete("/submissions/{id}/files/{file_id}", api.DeleteSubmissionFile(pool))

			// Flight data
			r.Put("/tourists/{id}/flight_data", api.UpdateFlightData(pool))
			r.Post("/tourists/{id}/flight_data/apply_to_subgroup", api.ApplyFlightDataToSubgroup(pool))

			// Document templates (per-org custom .docx override)
			r.Get("/templates/doverenost", api.GetDoverenostTemplateStatus(uploadsDir))
			r.Post("/templates/doverenost", api.UploadDoverenostTemplate(uploadsDir))
			r.Delete("/templates/doverenost", api.DeleteDoverenostTemplate(uploadsDir))
			r.Get("/templates/doverenost/download", api.DownloadDoverenostTemplate(uploadsDir))
		})
	})

	return r
}
