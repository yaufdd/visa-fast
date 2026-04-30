/* eslint-disable react-refresh/only-export-components */
// AddressesStep — wizard step 4. Registration + home address; "same as
// registration" checkbox mirrors the reg field into home and locks it.

import { makeFieldFactories } from '../fieldFactories';

export default function AddressesStep({ payload, setField, errors }) {
  const { textareaField } = makeFieldFactories({ payload, errors, setField });

  const sameAddress = !!payload.same_address;

  const onSameAddressToggle = (checked) => {
    setField('same_address', checked);
    if (checked) {
      // Mirror the reg address into home immediately.
      setField('home_address_ru', payload.reg_address_ru || '');
    }
  };

  // When the checkbox is checked, edits to reg_address_ru should propagate
  // to home_address_ru live. We can't override the textareaField factory
  // closure, so render the reg field manually here.
  const onRegChange = (e) => {
    const v = e.target.value;
    setField('reg_address_ru', v);
    if (sameAddress) setField('home_address_ru', v);
  };

  return (
    <div className="fw-step-content">
      <label className={`sf-field${errors.reg_address_ru ? ' has-error' : ''}`} data-field="reg_address_ru">
        <span className="sf-label">Адрес регистрации</span>
        <textarea
          value={payload.reg_address_ru ?? ''}
          onChange={onRegChange}
          rows={3}
        />
        {errors.reg_address_ru && <span className="sf-error">{errors.reg_address_ru}</span>}
      </label>

      <label className="sf-checkbox-row" data-field="same_address">
        <input
          type="checkbox"
          checked={sameAddress}
          onChange={(e) => onSameAddressToggle(e.target.checked)}
        />
        <span>Совпадает с адресом регистрации</span>
      </label>

      {sameAddress
        ? (
          <label className="sf-field" data-field="home_address_ru">
            <span className="sf-label">Домашний адрес *</span>
            <textarea
              value={payload.home_address_ru ?? ''}
              rows={3}
              disabled
            />
          </label>
        )
        : textareaField('home_address_ru', 'Домашний адрес *')
      }
    </div>
  );
}

export function validate(payload) {
  const errors = {};
  if (!String(payload.home_address_ru || '').trim()) {
    errors.home_address_ru = 'Укажите адрес фактического проживания';
  }
  return errors;
}
