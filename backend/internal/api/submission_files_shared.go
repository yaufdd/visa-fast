package api

import (
	"crypto/rand"
	"encoding/hex"
)

// Shared helpers + constants for submission-file handling. The public
// PublicSubmit handler (handlers_public.go) and the manager wizard
// handlers (admin_submission_uploads.go) both use these — keeping the
// declarations in one neutral file means neither side accidentally
// drifts from the other on the mime whitelist, the size cap, or the
// path-naming policy.

// maxSubmissionFileSize caps each individual scan upload at 50 MB. The
// public side is unauthenticated, so we keep this tighter than internal
// upload flows; passport / ticket / voucher scans are well under this in
// practice.
const maxSubmissionFileSize = 50 << 20

// allowedSubmissionFileTypes enumerates the file_type values accepted on
// upload endpoints. Mirrors the DB CHECK on submission_files.file_type.
var allowedSubmissionFileTypes = map[string]bool{
	"passport_internal": true,
	"passport_foreign":  true,
	"ticket":            true,
	"voucher":           true,
}

// multiFileTypes are the file_type values that allow MULTIPLE rows per
// submission. Tickets and vouchers are obviously stackable; the internal
// Russian passport joined the list when the manager wizard's Документы
// step was reworked — a manager often needs both the main page and the
// registration page on file. Foreign passport stays single (one row per
// submission, replace-on-upload via ON CONFLICT).
var multiFileTypes = map[string]bool{
	"passport_internal": true,
	"ticket":            true,
	"voucher":           true,
}

// randomFileSuffix returns 8 hex chars used to disambiguate filenames
// when multiple files of the same file_type live next to each other on
// disk under <uploadsDir>/<org>/submissions/<id>/.
func randomFileSuffix() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// submissionTmpName returns ".tmp.<8 hex>.<ext>" for use as a sibling tmp
// filename next to the final submission file. The dot prefix flags it as
// in-progress; the random suffix avoids collisions between concurrent
// uploads for the same (submission_id, file_type).
func submissionTmpName(ext string) (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ".tmp." + hex.EncodeToString(b[:]) + ext, nil
}
