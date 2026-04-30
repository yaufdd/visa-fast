// adminWizardAdapter — wires the FormWizard's six API hooks to the
// manager-side /api/submissions/* endpoints (session-authenticated).
// Mirror of publicWizardAdapter but pointed at the dashboard endpoints
// that landed in the 5aea440 → 0719bf8 commit chain.
//
// `persistEnabled: false` — admin mode treats the server as the source of
// truth. A manager may share the same browser with a colleague, switch
// orgs mid-session, or open multiple submissions in tabs; per-browser
// localStorage caching would only confuse things.

import {
  apiCreateSubmission,
  createDraftSubmission,
  deleteSubmissionFileAdmin,
  listSubmissionFilesAdmin,
  parseSubmissionPassportAdmin,
  parseSubmissionTicketAdmin,
  parseSubmissionVoucherAdmin,
  submissionFileDownloadUrl,
  updateSubmission,
  uploadSubmissionFileAdmin,
} from '../../api/client';

export function adminWizardAdapter() {
  return {
    persistEnabled: false,
    isPublic: false,
    // 'upload-now' — the manager has a real submission row from the
    // moment the page mounts (createDraftSubmission ran on /submissions/new),
    // so per-file uploads happen as soon as the picker returns. Public
    // mode is 'upload-on-submit' instead.
    filesMode: 'upload-now',
    startSubmission: () =>
      createDraftSubmission().then((data) => ({ submissionId: data.submission_id })),
    uploadFile: (submissionId, fileType, file, onProgress) =>
      uploadSubmissionFileAdmin(submissionId, fileType, file, onProgress),
    listFiles: (submissionId) => listSubmissionFilesAdmin(submissionId),
    deleteFile: (submissionId, fileId) =>
      deleteSubmissionFileAdmin(submissionId, fileId),
    parsePassport: (submissionId, fileId, type) =>
      parseSubmissionPassportAdmin(submissionId, fileId, type),
    parseTicket: (submissionId, fileId) =>
      parseSubmissionTicketAdmin(submissionId, fileId),
    parseVoucher: (submissionId, fileId) =>
      parseSubmissionVoucherAdmin(submissionId, fileId),
    // Manager-side has cookie-authenticated GET on the file blob; the
    // public form doesn't expose this (the tourist holds the source PDF
    // on their own device). When omitted, the download icon is hidden.
    downloadUrl: (submissionId, fileId) =>
      submissionFileDownloadUrl(submissionId, fileId),
    // Final submit on the manager side: branch on whether we have a
    // submissionId already.
    //
    //   - With id  → PUT /api/submissions/{id}. The backend's
    //                UpdateSubmission handler flips status='draft' to
    //                'pending' on the same UPDATE (see the matching
    //                backend tweak landed alongside this adapter), so a
    //                row that started as a draft gets promoted as part
    //                of the same call. Rows already in 'pending' or
    //                'attached' keep their status.
    //
    //   - Without id → POST /api/submissions (CreateSubmissionByManager).
    //                Used as a defensive fallback; the dashboard now
    //                always allocates a draft on mount, but if that
    //                allocation failed for some reason we still want the
    //                submit to succeed.
    submit: (submissionId, payload, consentAccepted) => {
      if (submissionId) {
        return updateSubmission(submissionId, payload).then(() => ({ id: submissionId }));
      }
      return apiCreateSubmission(payload, consentAccepted);
    },
  };
}
