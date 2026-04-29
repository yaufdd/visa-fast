/* eslint-disable react-refresh/only-export-components */
// TravelStep — public-form-only step: history of travel + criminal record.
// File uploads moved to a separate DocumentsStep so the public form
// matches the user's mental split (data ↔ scans).

import { makeFieldFactories } from '../fieldFactories';

export default function TravelStep({ payload, setField, errors }) {
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
    </div>
  );
}

export function validate() {
  // No required fields here — selects have defaults; previous_visits is
  // only revealed when "Да" is chosen and is informational.
  return {};
}
