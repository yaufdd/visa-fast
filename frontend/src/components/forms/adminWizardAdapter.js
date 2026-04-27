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
  updateSubmission,
  uploadSubmissionFileAdmin,
} from '../../api/client';

export function adminWizardAdapter() {
  return {
    persistEnabled: false,
    startSubmission: () =>
      createDraftSubmission().then((data) => ({ submissionId: data.submission_id })),
    uploadFile: (submissionId, fileType, file, onProgress) =>
      uploadSubmissionFileAdmin(submissionId, fileType, file, onProgress),
    listFiles: (submissionId) => listSubmissionFilesAdmin(submissionId),
    deleteFile: (submissionId, fileId) =>
      deleteSubmissionFileAdmin(submissionId, fileId),
    parsePassport: (submissionId, fileId, type) =>
      parseSubmissionPassportAdmin(submissionId, fileId, type),
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
