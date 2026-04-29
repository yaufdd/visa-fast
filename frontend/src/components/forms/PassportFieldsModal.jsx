// PassportFieldsModal — manager-side modal for reviewing / editing the
// internal passport fields (series, number, issue date, "issued by") on
// the submission. Opens from the «✎ Открыть» button under the
// Документы → Скан внутреннего паспорта section.
//
// Fields are wired directly to the wizard payload via setField, so any
// edit here is mirrored elsewhere (e.g. ReviewStep) without an extra
// commit step.

import Modal from '../Modal';
import { makeFieldFactories, InternalPassportField } from './fieldFactories';

export default function PassportFieldsModal({
  open, onClose, payload, errors, setField,
}) {
  const { boxedField, dateField } = makeFieldFactories({ payload, errors, setField });

  return (
    <Modal open={open} onClose={onClose} title="Внутренний паспорт" width={520}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <InternalPassportField payload={payload} errors={errors} setField={setField} />
        {dateField('internal_issued_ru', 'Дата выдачи')}
        {boxedField('internal_issued_by_ru', 'Кем выдан')}
        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 8 }}>
          <button type="button" className="btn btn-primary btn-sm" onClick={onClose}>
            Готово
          </button>
        </div>
      </div>
    </Modal>
  );
}
