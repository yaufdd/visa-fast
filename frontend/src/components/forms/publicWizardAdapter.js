// publicWizardAdapter — wires the FormWizard's six API hooks to the
// public-slug endpoints used by the tourist-facing /form/<slug> page.
//
// The wizard treats this as an opaque adapter so the manager-side
// dashboard can supply its own session-authenticated implementation
// without the wizard needing to know about slugs at all.
//
// `persistEnabled: true` keeps the localStorage draft-restore behaviour
// the public form has shipped with since 9dfcb59 — the tourist may
// close their tab mid-anketa and we want to bring them back.

import {
  startSubmission,
  uploadSubmissionFile,
  listSubmissionFiles,
  deleteSubmissionFile,
  parsePassport,
} from '../../api/files';
import { publicCreateSubmission } from '../../api/client';

export function publicWizardAdapter(slug) {
  return {
    persistEnabled: true,
    isPublic: true,
    startSubmission: () =>
      startSubmission(slug).then((data) => ({ submissionId: data.submission_id })),
    uploadFile: (submissionId, fileType, file, onProgress) =>
      uploadSubmissionFile(slug, submissionId, fileType, file, onProgress),
    listFiles: (submissionId) => listSubmissionFiles(slug, submissionId),
    deleteFile: (submissionId, fileId) =>
      deleteSubmissionFile(slug, submissionId, fileId),
    parsePassport: (submissionId, fileId, type) =>
      parsePassport(slug, submissionId, fileId, type),
    // Final submit on the public form: when /start succeeded we pass the
    // existing draft id so the backend finalises it (preserving any
    // attached files). Without an id the backend creates a fresh row.
    submit: (submissionId, payload, consentAccepted) =>
      publicCreateSubmission(slug, payload, consentAccepted, submissionId || undefined),
  };
}
