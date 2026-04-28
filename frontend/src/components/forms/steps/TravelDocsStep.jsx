/* eslint-disable react-refresh/only-export-components */
// TravelDocsStep — wizard step 6. Travel history + criminal record +
// ticket / voucher uploads (no auto-fill — managers run the parsers later).
// Phone field moved to PersonalStep (Step 1) — see commit history.

import FileUploadField from '../FileUploadField';
import FileMultiUploadField from '../FileMultiUploadField';
import { makeFieldFactories } from '../fieldFactories';

export default function TravelDocsStep({
  payload, setField, errors, files, setFiles, adapter, submissionId,
}) {
  const { selectField, textareaField } = makeFieldFactories({ payload, errors, setField });
  const showPreviousVisits = payload.been_to_japan_ru === 'Да';

  return (
    <div className="fw-step-content">
      {selectField('been_to_japan_ru', 'Был ли в Японии', [
        { value: 'Нет', label: 'Нет' },
        { value: 'Да', label: 'Да' },
      ])}

      {showPreviousVisits && textareaField('previous_visits_ru', 'Даты прошлых визитов')}

      {selectField('criminal_record_ru', 'Была ли судимость', [
        { value: 'Нет', label: 'Нет' },
        { value: 'Да', label: 'Да' },
      ])}

      <FileMultiUploadField
        label="Авиабилеты"
        fileType="ticket"
        adapter={adapter}
        submissionId={submissionId}
        currentFiles={Array.isArray(files.ticket) ? files.ticket : []}
        onAdded={(meta) => setFiles((f) => ({
          ...f,
          ticket: [...(Array.isArray(f.ticket) ? f.ticket : []), meta],
        }))}
        onRemoved={(fileId) => setFiles((f) => ({
          ...f,
          ticket: (Array.isArray(f.ticket) ? f.ticket : []).filter((x) => x.id !== fileId),
        }))}
        acceptMime="application/pdf,image/jpeg,image/png"
      />

      <FileMultiUploadField
        label="Ваучеры на отели"
        fileType="voucher"
        adapter={adapter}
        submissionId={submissionId}
        currentFiles={Array.isArray(files.voucher) ? files.voucher : []}
        onAdded={(meta) => setFiles((f) => ({
          ...f,
          voucher: [...(Array.isArray(f.voucher) ? f.voucher : []), meta],
        }))}
        onRemoved={(fileId) => setFiles((f) => ({
          ...f,
          voucher: (Array.isArray(f.voucher) ? f.voucher : []).filter((x) => x.id !== fileId),
        }))}
        acceptMime="application/pdf,image/jpeg,image/png"
      />
    </div>
  );
}

export function validate() {
  // Phone validation moved to PersonalStep where the field now lives.
  // Travel-docs step has no required fields — uploads are optional.
  return {};
}
