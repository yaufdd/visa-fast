/* eslint-disable react-refresh/only-export-components */
// ForeignPassportStep — wizard step 3. Foreign (travel) passport.

import FileUploadField from '../FileUploadField';
import { makeFieldFactories, PassportNumberField } from '../fieldFactories';
import { makePassportAutoFillHandler } from '../passportAutoFill';

export default function ForeignPassportStep({
  payload, setField, errors, files, setFiles, adapter, submissionId,
  setPayload, setAutoFillNotice, autoFillNotice, filesMode,
}) {
  const { boxedField, dateField, selectField } = makeFieldFactories({ payload, errors, setField });

  const isAdmin = !adapter?.isPublic;
  // Recognition only makes sense in upload-now (admin) mode where the
  // scan already lives on the server. The public form ships everything
  // in one final POST, so there's nothing to OCR mid-form.
  const onAutoFill = isAdmin
    ? makePassportAutoFillHandler(setPayload, setAutoFillNotice, 'foreign')
    : null;

  return (
    <div className="fw-step-content">
      <PassportNumberField payload={payload} errors={errors} setField={setField} />

      {selectField('passport_type_ru', 'Тип паспорта', [
        { value: 'Обычный', label: 'Обычный' },
        { value: 'Дипломатический', label: 'Дипломатический' },
        { value: 'Служебный', label: 'Служебный' },
      ])}
      {dateField('issue_date', 'Дата выдачи')}
      {dateField('expiry_date', 'Дата окончания')}
      {boxedField('issued_by_ru', 'Кем выдан')}

      {isAdmin && (
        <FileUploadField
          label="Скан загранпаспорта"
          fileType="passport_foreign"
          adapter={adapter}
          submissionId={submissionId}
          currentFile={files.passport_foreign || null}
          onUploaded={(meta) => setFiles((f) => ({ ...f, passport_foreign: meta }))}
          onDeleted={() => setFiles((f) => {
            const next = { ...f };
            next.passport_foreign = null;
            return next;
          })}
          showDelete={Boolean(adapter?.isPublic)}
          onAutoFill={onAutoFill}
          parseType="foreign"
          acceptMime="application/pdf,image/jpeg,image/png"
          filesMode={filesMode}
        />
      )}

      {autoFillNotice && (
        <div className="sf-autofill-notice">{autoFillNotice}</div>
      )}
    </div>
  );
}

export function validate(payload) {
  const errors = {};
  const passNum = (payload.passport_number || '').trim();
  if (!passNum) {
    errors.passport_number = 'Укажите номер загранпаспорта';
  } else if (passNum.length !== 9) {
    errors.passport_number = 'Должно быть 9 символов';
  }
  if (!String(payload.issue_date || '').trim()) {
    errors.issue_date = 'Укажите дату выдачи';
  }
  if (!String(payload.expiry_date || '').trim()) {
    errors.expiry_date = 'Укажите дату окончания';
  }
  return errors;
}
