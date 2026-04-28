/* eslint-disable react-refresh/only-export-components */
// InternalPassportStep — wizard step 2. Russian internal passport fields
// + scan upload widget with auto-fill.

import FileUploadField from '../FileUploadField';
import { makeFieldFactories, InternalPassportField } from '../fieldFactories';
import { makePassportAutoFillHandler } from '../passportAutoFill';

export default function InternalPassportStep({
  payload, setField, errors, files, setFiles, adapter, submissionId,
  setPayload, setAutoFillNotice, autoFillNotice,
}) {
  const { boxedField, dateField } = makeFieldFactories({ payload, errors, setField });

  const onAutoFill = makePassportAutoFillHandler(setPayload, setAutoFillNotice, 'internal');

  return (
    <div className="fw-step-content">
      <InternalPassportField payload={payload} errors={errors} setField={setField} />

      {dateField('internal_issued_ru', 'Дата выдачи')}
      {boxedField('internal_issued_by_ru', 'Кем выдан')}

      {/* Scan upload — placed below the manual fields so a tourist who
          already typed everything still sees a clear "or upload a scan"
          affordance. */}
      <FileUploadField
        label="Скан внутреннего паспорта"
        fileType="passport_internal"
        adapter={adapter}
        submissionId={submissionId}
        currentFile={files.passport_internal || null}
        onUploaded={(meta) => setFiles((f) => ({ ...f, passport_internal: meta }))}
        onDeleted={() => setFiles((f) => {
          const next = { ...f };
          next.passport_internal = null;
          return next;
        })}
        onAutoFill={onAutoFill}
        parseType="internal"
        acceptMime="application/pdf,image/jpeg,image/png"
      />

      {autoFillNotice && (
        <div className="sf-autofill-notice">{autoFillNotice}</div>
      )}
    </div>
  );
}

export function validate(payload) {
  const errors = {};
  const series = (payload.internal_series || '').trim();
  if (series && !/^\d{4}$/.test(series)) {
    errors.internal_series = 'Должно быть ровно 4 цифры';
  }
  const intNum = (payload.internal_number || '').trim();
  if (intNum && !/^\d{6}$/.test(intNum)) {
    errors.internal_number = 'Должно быть ровно 6 цифр';
  }
  return errors;
}
