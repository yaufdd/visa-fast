// publicWizardAdapter — adapter for the tourist-facing /form/<slug> page.
//
// The wizard treats this as an opaque adapter so the manager-side
// dashboard can supply its own session-authenticated implementation
// without the wizard needing to know about slugs at all.
//
// Public mode is now "single-shot": the tourist picks files locally,
// fills the form, and on final submit we ship payload + files in one
// multipart POST. There is no draft row created server-side during
// typing, so the adapter only exposes what the public flow actually
// needs:
//
//   - `filesMode: 'upload-on-submit'` tells FormWizard / the file
//      widgets to keep selected files in component state instead of
//      uploading per-pick.
//   - `submit(payload, consentAccepted, files)` posts the multipart body
//      to /api/public/submissions/<slug>. On 201 the wizard's caller
//      handles navigation; on 409 the same Error message bubbles up so
//      the existing "duplicate submission" banner still works.
//
// Recognition (parsePassport / parseTicket / parseVoucher) is admin-only
// now — public-mode users haven't uploaded anything to the backend yet,
// so there is nothing to OCR. The DocumentsStep / ForeignPassportStep
// components already gate those buttons on `!adapter.isPublic`.
//
// `persistEnabled: true` keeps the localStorage draft-restore behaviour
// for typed text — the tourist may close the tab mid-anketa and we want
// to bring their text back. File picks do not survive a reload (File
// objects can't be serialised) and that's documented to the user via
// the absence of a "✓ uploaded" badge after restore.

import { publicSubmit } from '../../api/client';

export function publicWizardAdapter(slug) {
  return {
    persistEnabled: true,
    isPublic: true,
    // 'upload-on-submit' — file widgets keep File objects in state and
    // hand them up at submit time. Admin uses 'upload-now'.
    filesMode: 'upload-on-submit',
    submit: (payload, consentAccepted, files, captchaToken) =>
      publicSubmit(slug, payload, consentAccepted, files, captchaToken),
  };
}
