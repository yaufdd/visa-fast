/* eslint-disable react-refresh/only-export-components */
// fieldFactories.jsx — shared input renderers used by SubmissionForm (manager
// edit view) and the public-form wizard step components. The factories are
// JSX-returning helpers, not React components, so each call site can pass in
// its own `payload`, `errors`, and `setField` closures and reuse the same
// look-and-feel without lifting state.
//
// Each factory takes a context object:
//   { payload, errors, setField }
// and returns a function (name, label, extra?) → JSX. This keeps call-sites
// idiomatic — `textField('name_cyr', 'ФИО кириллицей')` reads exactly the
// same as before the extraction.

import { useId } from 'react';
import BoxedCharInput from '../BoxedCharInput';
import { dmyToIso, isoToDmy } from '../../utils/dates';
import { normalizePhone, phoneOnInput } from '../../utils/phone';

// Allowed characters in "кем выдан" — Cyrillic/Latin letters, digits, spaces
// and common passport-code punctuation (dash, slash, dot, comma, №). No
// length limit.
export const ISSUED_BY_SANITIZE = (s) =>
  s.replace(/[^a-zA-Zа-яА-ЯёЁ0-9 №.,/-]/g, '');

export function sanitizeLatin(value) {
  return value.toUpperCase().replace(/[^A-Z\s]/g, '');
}

export function makeFieldFactories({ payload, errors, setField }) {
  const handleDateChange = (name) => (e) => {
    setField(name, isoToDmy(e.target.value));
  };

  const handlePhoneBlur = (name) => (e) => {
    const normalized = normalizePhone(e.target.value);
    if (normalized !== e.target.value) setField(name, normalized);
  };

  const textField = (name, label, extra = {}) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <input
          type="text"
          value={payload[name] ?? ''}
          onChange={(e) => setField(name, e.target.value)}
          placeholder={extra.placeholder || ''}
          autoComplete="off"
        />
        {extra.hint && !err && <span className="sf-hint">{extra.hint}</span>}
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  const boxedField = (name, label) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <BoxedCharInput
          value={payload[name] ?? ''}
          onChange={(v) => setField(name, v)}
          sanitize={ISSUED_BY_SANITIZE}
          ariaLabel={label}
        />
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  const textareaField = (name, label, extra = {}) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <textarea
          value={payload[name] ?? ''}
          onChange={(e) => setField(name, e.target.value)}
          rows={extra.rows || 3}
          disabled={extra.disabled || false}
        />
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  const selectField = (name, label, options, extra = {}) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <select
          value={payload[name] ?? ''}
          onChange={(e) => setField(name, e.target.value)}
        >
          {options.map((opt) => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
        {extra.hint && !err && <span className="sf-hint">{extra.hint}</span>}
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  const dateField = (name, label) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <input
          type="date"
          value={dmyToIso(payload[name] ?? '')}
          onChange={handleDateChange(name)}
        />
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  const phoneField = (name, label) => {
    const err = errors[name];
    return (
      <label className={`sf-field${err ? ' has-error' : ''}`} data-field={name}>
        <span className="sf-label">{label}</span>
        <input
          type="tel"
          value={payload[name] ?? ''}
          onChange={(e) => setField(name, phoneOnInput(e.target.value))}
          onBlur={handlePhoneBlur(name)}
          autoComplete="off"
        />
        {err && <span className="sf-error">{err}</span>}
      </label>
    );
  };

  return { textField, boxedField, textareaField, selectField, dateField, phoneField };
}

// PassportNumberField — segmented 2+7 digit foreign passport number.
// Stored in payload.passport_number as the concatenated 9-digit string.
// Extracted as a real component (not factory closure) because it needs
// stable element IDs for focus management.
export function PassportNumberField({ payload, errors, setField }) {
  const baseId = useId();
  const part1Id = `${baseId}-p1`;
  const part2Id = `${baseId}-p2`;
  const err = errors.passport_number;
  const raw = (payload.passport_number || '').replace(/\D/g, '').slice(0, 9);
  const part1 = raw.slice(0, 2);
  const part2 = raw.slice(2, 9);

  const setCombined = (p1, p2) => {
    setField('passport_number', (p1 + p2).slice(0, 9));
  };

  const onFirst = (e) => {
    const v = e.target.value.replace(/\D/g, '').slice(0, 2);
    setCombined(v, part2);
    if (v.length === 2) {
      const next = document.getElementById(part2Id);
      if (next) next.focus();
    }
  };

  const onSecond = (e) => {
    const v = e.target.value.replace(/\D/g, '').slice(0, 7);
    setCombined(part1, v);
  };

  const onSecondKey = (e) => {
    if (e.key === 'Backspace' && !part2) {
      const prev = document.getElementById(part1Id);
      if (prev) prev.focus();
    }
  };

  return (
    <label className={`sf-field${err ? ' has-error' : ''}`} data-field="passport_number">
      <span className="sf-label">Номер загранпаспорта</span>
      <div className="sf-passport-input">
        <div className="sf-passport-col">
          <input
            id={part1Id}
            type="text"
            inputMode="numeric"
            value={part1}
            onChange={onFirst}
            maxLength={2}
            className="sf-passport-seg sf-passport-seg--short"
            autoComplete="off"
          />
          <span className="sf-passport-sublabel">Серия</span>
        </div>
        <div className="sf-passport-col">
          <input
            id={part2Id}
            type="text"
            inputMode="numeric"
            value={part2}
            onChange={onSecond}
            onKeyDown={onSecondKey}
            maxLength={7}
            className="sf-passport-seg sf-passport-seg--long"
            autoComplete="off"
          />
          <span className="sf-passport-sublabel">Номер</span>
        </div>
      </div>
      {err && <span className="sf-error">{err}</span>}
    </label>
  );
}

// InternalPassportField — segmented 4-digit series + 6-digit number,
// stored as separate fields internal_series / internal_number.
export function InternalPassportField({ payload, errors, setField }) {
  const baseId = useId();
  const seriesId = `${baseId}-series`;
  const numberId = `${baseId}-number`;
  const serErr = errors.internal_series;
  const numErr = errors.internal_number;
  const err = serErr || numErr;
  const series = (payload.internal_series || '').replace(/\D/g, '').slice(0, 4);
  const number = (payload.internal_number || '').replace(/\D/g, '').slice(0, 6);

  const onSeries = (e) => {
    const v = e.target.value.replace(/\D/g, '').slice(0, 4);
    setField('internal_series', v);
    if (v.length === 4) {
      const next = document.getElementById(numberId);
      if (next) next.focus();
    }
  };

  const onNumber = (e) => {
    const v = e.target.value.replace(/\D/g, '').slice(0, 6);
    setField('internal_number', v);
  };

  const onNumberKey = (e) => {
    if (e.key === 'Backspace' && !number) {
      const prev = document.getElementById(seriesId);
      if (prev) prev.focus();
    }
  };

  return (
    <label className={`sf-field${err ? ' has-error' : ''}`} data-field="internal_series">
      <span className="sf-label">Серия и номер</span>
      <div className="sf-passport-input">
        <div className="sf-passport-col">
          <input
            id={seriesId}
            type="text"
            inputMode="numeric"
            value={series}
            onChange={onSeries}
            maxLength={4}
            className="sf-passport-seg sf-passport-seg--series"
            autoComplete="off"
          />
          <span className="sf-passport-sublabel">Серия</span>
        </div>
        <div className="sf-passport-col">
          <input
            id={numberId}
            type="text"
            inputMode="numeric"
            value={number}
            onChange={onNumber}
            onKeyDown={onNumberKey}
            maxLength={6}
            className="sf-passport-seg sf-passport-seg--number"
            autoComplete="off"
          />
          <span className="sf-passport-sublabel">Номер</span>
        </div>
      </div>
      {serErr && <span className="sf-error">{serErr}</span>}
      {numErr && !serErr && <span className="sf-error">{numErr}</span>}
    </label>
  );
}
