/* eslint-disable react-refresh/only-export-components */
// TravelDocsStep — wizard step 6. Travel history + criminal record + phone +
// ticket / voucher uploads (no auto-fill — managers run the parsers later).

import FileUploadField from '../FileUploadField';
import { makeFieldFactories } from '../fieldFactories';

export default function TravelDocsStep({
  payload, setField, errors, files, setFiles, adapter, submissionId,
}) {
  const { selectField, textareaField, phoneField } = makeFieldFactories({ payload, errors, setField });
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

      {phoneField('phone', 'Телефон')}

      <FileUploadField
        label="Авиабилет(ы)"
        fileType="ticket"
        adapter={adapter}
        submissionId={submissionId}
        currentFile={files.ticket || null}
        onUploaded={(meta) => setFiles((f) => ({ ...f, ticket: meta }))}
        onDeleted={() => setFiles((f) => {
          const next = { ...f };
          next.ticket = null;
          return next;
        })}
        acceptMime="application/pdf,image/jpeg,image/png"
      />
      <span className="sf-hint">
        Менеджер распознает скан автоматически после прикрепления к туру.
      </span>

      <FileUploadField
        label="Ваучер на отель(и)"
        fileType="voucher"
        adapter={adapter}
        submissionId={submissionId}
        currentFile={files.voucher || null}
        onUploaded={(meta) => setFiles((f) => ({ ...f, voucher: meta }))}
        onDeleted={() => setFiles((f) => {
          const next = { ...f };
          next.voucher = null;
          return next;
        })}
        acceptMime="application/pdf,image/jpeg,image/png"
      />
      <span className="sf-hint">
        Менеджер распознает скан автоматически после прикрепления к туру.
      </span>
    </div>
  );
}

export function validate(payload) {
  const errors = {};
  // Phone required for the visa anketa — keeping it required on the
  // wizard submit (legacy form accepted empty, but the visa form needs it).
  if (!String(payload.phone || '').trim()) {
    errors.phone = 'Укажите контактный телефон';
  }
  return errors;
}
