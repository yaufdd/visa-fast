/* eslint-disable react-refresh/only-export-components */
// DocumentsStep — uploads + (admin only) recognition + manual edit modals.
//
// Each of the three sections (внутренний паспорт, авиабилеты, ваучеры) has
// the same shape: file rows on top with their replace / delete affordances,
// then an action row at the bottom carrying the magic-recognise button and
// an «✎ Открыть» button that pops a modal where the manager can review or
// type the data by hand.
//
// Storage:
//   - паспорт   → payload.internal_series / internal_number / internal_issued_ru / internal_issued_by_ru
//   - билеты    → payload.flight_data  (object with arrival / departure)
//   - ваучеры   → payload.hotels       (array of { hotel_name, city, address, check_in, check_out })
//
// Public mode (tourist) hides every action button — only the upload widgets.

import { useState } from 'react';
import FileMultiUploadField from '../FileMultiUploadField';
import { makePassportAutoFillHandler } from '../passportAutoFill';
import PassportFieldsModal from '../PassportFieldsModal';
import HotelsListModal from '../HotelsListModal';
import FlightDataForm from '../../FlightDataForm';

function MagicIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M8 1.5L9.2 6.8L14.5 8L9.2 9.2L8 14.5L6.8 9.2L1.5 8L6.8 6.8Z"
        fill="currentColor"/>
    </svg>
  );
}

function EyeIcon({ size = 14 }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none">
      <path d="M1.5 8s2.5-4.5 6.5-4.5 6.5 4.5 6.5 4.5-2.5 4.5-6.5 4.5S1.5 8 1.5 8z"
        stroke="currentColor" strokeWidth="1.3" strokeLinejoin="round"/>
      <circle cx="8" cy="8" r="1.8" stroke="currentColor" strokeWidth="1.3"/>
    </svg>
  );
}

function safeParse(raw) {
  if (!raw) return null;
  if (typeof raw === 'object') return raw;
  try { return JSON.parse(raw); } catch { return null; }
}

// Action row reused at the bottom of every section.
function SectionActions({ children }) {
  return (
    <div style={{ display: 'flex', gap: 8, marginTop: 8, flexWrap: 'wrap' }}>
      {children}
    </div>
  );
}

export default function DocumentsStep({
  payload, setField, errors, files, setFiles, adapter, submissionId,
  setPayload, autoFillNotice, setAutoFillNotice, filesMode,
}) {
  const isAdmin = !adapter?.isPublic;

  const [passportModalOpen, setPassportModalOpen] = useState(false);
  const [flightModalOpen, setFlightModalOpen] = useState(false);
  const [hotelsModalOpen, setHotelsModalOpen] = useState(false);
  // Three independent flags — each "open" eye-button winks (twice every
  // 2s) until the manager clicks it, signalling that fresh recognised
  // data is waiting inside. Cleared in the button's onClick.
  const [winkPassport, setWinkPassport] = useState(false);
  const [winkFlight, setWinkFlight] = useState(false);
  const [winkHotels, setWinkHotels] = useState(false);

  // Admin-only: recognition autofill writes the parsed passport fields
  // straight into the wizard payload (and surfaces a transient
  // «распознано» notice).
  const onPassportAutoFill = isAdmin
    ? makePassportAutoFillHandler(setPayload, setAutoFillNotice, 'internal')
    : null;

  const [parsingPassport, setParsingPassport] = useState(false);
  const [passportParseError, setPassportParseError] = useState(null);
  // Combine both UI slots — admin recognition button processes all pages.
  const passportFiles = [
    ...(Array.isArray(files.passport_main) ? files.passport_main : []),
    ...(Array.isArray(files.passport_reg) ? files.passport_reg : []),
    ...(Array.isArray(files.passport_internal) ? files.passport_internal : []),
  ];
  const handleParsePassport = async () => {
    if (passportFiles.length === 0 || !onPassportAutoFill) return;
    setParsingPassport(true);
    setPassportParseError(null);
    try {
      // Multiple scans (main page + registration page) each carry partial
      // info — parse oldest-first so a later scan can overwrite an earlier
      // empty field but won't blank out fields that an earlier scan filled.
      for (const f of passportFiles) {
        const fields = await adapter.parsePassport(submissionId, f.id, 'internal');
        onPassportAutoFill(fields);
      }
      setWinkPassport(true);
    } catch (e) {
      setPassportParseError(e.message || 'Не удалось распознать паспорт');
    } finally {
      setParsingPassport(false);
    }
  };

  // payload.flight_data is stored as an object once edited from this modal,
  // but might still be a JSON string if it came from somewhere upstream.
  const flightInitial = safeParse(payload.flight_data) || {};
  const handleSaveFlight = async (data) => {
    setPayload((p) => ({ ...p, flight_data: data }));
  };

  const hotelsValue = Array.isArray(payload.hotels) ? payload.hotels : [];
  const handleSaveHotels = (next) => {
    setPayload((p) => ({ ...p, hotels: next }));
  };

  // ── Ticket recognition ───────────────────────────────────────────────
  const ticketFiles = Array.isArray(files.ticket) ? files.ticket : [];
  const [parsingTicket, setParsingTicket] = useState(false);
  const [ticketParseError, setTicketParseError] = useState(null);
  const handleParseTicket = async () => {
    if (ticketFiles.length === 0) return;
    setParsingTicket(true);
    setTicketParseError(null);
    try {
      // Several tickets per submission are unusual but possible (multi-leg
      // trip with separate eTicket emails). Parse them in order; the last
      // result wins for each leg, matching the per-tourist parser
      // semantics on the groups page.
      let merged = safeParse(payload.flight_data) || {};
      for (const f of ticketFiles) {
        const data = await adapter.parseTicket(submissionId, f.id);
        merged = { ...merged, ...data };
      }
      setPayload((p) => ({ ...p, flight_data: merged }));
      setWinkFlight(true);
    } catch (e) {
      setTicketParseError(e.message || 'Не удалось распознать билет');
    } finally {
      setParsingTicket(false);
    }
  };

  // ── Voucher recognition ──────────────────────────────────────────────
  const voucherFiles = Array.isArray(files.voucher) ? files.voucher : [];
  const [parsingVoucher, setParsingVoucher] = useState(false);
  const [voucherParseError, setVoucherParseError] = useState(null);
  const handleParseVoucher = async () => {
    if (voucherFiles.length === 0) return;
    setParsingVoucher(true);
    setVoucherParseError(null);
    try {
      // Each voucher returns its own list of hotel stays — append them
      // together rather than picking one. The manager can deduplicate /
      // tweak via the «✎ Открыть» modal.
      const accumulated = [];
      for (const f of voucherFiles) {
        const arr = await adapter.parseVoucher(submissionId, f.id);
        if (Array.isArray(arr)) accumulated.push(...arr);
      }
      setPayload((p) => ({
        ...p,
        hotels: [
          ...(Array.isArray(p.hotels) ? p.hotels : []),
          ...accumulated.map((h) => ({
            hotel_name: h.hotel_name || h.name_en || '',
            city: h.city || '',
            address: h.address || '',
            check_in: h.check_in || '',
            check_out: h.check_out || '',
          })),
        ],
      }));
      setWinkHotels(true);
    } catch (e) {
      setVoucherParseError(e.message || 'Не удалось распознать ваучер');
    } finally {
      setParsingVoucher(false);
    }
  };

  return (
    <div className="fw-step-content">
      {/* ── Внутренний паспорт — только публичная форма ───────────── */}
      {!isAdmin && (
        <>
          <FileMultiUploadField
            label="Первая страница паспорта"
            fileType="passport_main"
            adapter={adapter}
            submissionId={submissionId}
            currentFiles={Array.isArray(files.passport_main) ? files.passport_main : []}
            onAdded={(meta) => setFiles((f) => ({
              ...f,
              passport_main: [...(Array.isArray(f.passport_main) ? f.passport_main : []), meta],
            }))}
            onRemoved={(fileId) => setFiles((f) => ({
              ...f,
              passport_main: (Array.isArray(f.passport_main) ? f.passport_main : []).filter((x) => x.id !== fileId),
            }))}
            acceptMime="application/pdf,image/jpeg,image/png"
            compact
            showDelete={false}
            filesMode={filesMode}
          />
          <FileMultiUploadField
            label="Страница с регистрацией"
            fileType="passport_reg"
            adapter={adapter}
            submissionId={submissionId}
            currentFiles={Array.isArray(files.passport_reg) ? files.passport_reg : []}
            onAdded={(meta) => setFiles((f) => ({
              ...f,
              passport_reg: [...(Array.isArray(f.passport_reg) ? f.passport_reg : []), meta],
            }))}
            onRemoved={(fileId) => setFiles((f) => ({
              ...f,
              passport_reg: (Array.isArray(f.passport_reg) ? f.passport_reg : []).filter((x) => x.id !== fileId),
            }))}
            acceptMime="application/pdf,image/jpeg,image/png"
            compact
            showDelete={false}
            filesMode={filesMode}
          />
          <p className="sf-hint" style={{ marginTop: 4 }}>
            Страница с актуальной регистрацией. Если в паспорте есть страницы с предыдущими регистрациями — приложите их тоже.
          </p>
        </>
      )}
      {isAdmin && autoFillNotice && (
        <div className="sf-autofill-notice">{autoFillNotice}</div>
      )}
      {isAdmin && passportParseError && (
        <div className="error-message" style={{ marginTop: 6 }}>{passportParseError}</div>
      )}
      {isAdmin && (
        <SectionActions>
          <button
            type="button"
            className="btn btn-sm btn-magic"
            onClick={handleParsePassport}
            disabled={passportFiles.length === 0 || parsingPassport}
            title={passportFiles.length > 0
              ? 'Распознать сканы паспорта и заполнить поля'
              : 'Загрузите хотя бы один скан паспорта, чтобы запустить распознавание'}
            aria-label="Распознать"
            style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
          >
            {parsingPassport ? <span className="spinner" /> : <MagicIcon />}
          </button>
          <button
            type="button"
            className={`btn btn-secondary btn-sm${winkPassport ? ' btn-wink' : ''}`}
            onClick={() => { setWinkPassport(false); setPassportModalOpen(true); }}
            title="Просмотреть и отредактировать поля паспорта"
            aria-label="Открыть"
            style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
          >
            <EyeIcon />
          </button>
        </SectionActions>
      )}

      {/* ── Авиабилеты ─────────────────────────────────────────────── */}
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
        compact
        showDelete={!isAdmin}
        filesMode={filesMode}
      />
      {isAdmin && ticketParseError && (
        <div className="error-message" style={{ marginTop: 6 }}>{ticketParseError}</div>
      )}
      {isAdmin && (
        <SectionActions>
          <button
            type="button"
            className="btn btn-sm btn-magic"
            onClick={handleParseTicket}
            disabled={ticketFiles.length === 0 || parsingTicket}
            title={ticketFiles.length > 0
              ? 'Распознать сканы билетов и заполнить рейсы'
              : 'Загрузите хотя бы один скан билета, чтобы запустить распознавание'}
            aria-label="Распознать"
            style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
          >
            {parsingTicket ? <span className="spinner" /> : <MagicIcon />}
          </button>
          <button
            type="button"
            className={`btn btn-secondary btn-sm${winkFlight ? ' btn-wink' : ''}`}
            onClick={() => { setWinkFlight(false); setFlightModalOpen(true); }}
            title="Просмотреть и отредактировать рейсы"
            aria-label="Открыть"
            style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
          >
            <EyeIcon />
          </button>
        </SectionActions>
      )}

      {/* ── Ваучеры ────────────────────────────────────────────────── */}
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
        compact
        showDelete={!isAdmin}
        filesMode={filesMode}
      />
      {isAdmin && voucherParseError && (
        <div className="error-message" style={{ marginTop: 6 }}>{voucherParseError}</div>
      )}
      {isAdmin && (
        <SectionActions>
          <button
            type="button"
            className="btn btn-sm btn-magic"
            onClick={handleParseVoucher}
            disabled={voucherFiles.length === 0 || parsingVoucher}
            title={voucherFiles.length > 0
              ? 'Распознать сканы ваучеров и собрать список отелей'
              : 'Загрузите хотя бы один скан ваучера, чтобы запустить распознавание'}
            aria-label="Распознать"
            style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
          >
            {parsingVoucher ? <span className="spinner" /> : <MagicIcon />}
          </button>
          <button
            type="button"
            className={`btn btn-secondary btn-sm${winkHotels ? ' btn-wink' : ''}`}
            onClick={() => { setWinkHotels(false); setHotelsModalOpen(true); }}
            title="Просмотреть и отредактировать список отелей"
            aria-label="Открыть"
            style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '4px 8px' }}
          ><EyeIcon />
          </button>
        </SectionActions>
      )}

      {/* Modals — admin only */}
      {isAdmin && (
        <>
          <PassportFieldsModal
            open={passportModalOpen}
            onClose={() => setPassportModalOpen(false)}
            payload={payload}
            errors={errors}
            setField={setField}
          />
          <FlightDataForm
            open={flightModalOpen}
            initial={flightInitial}
            onClose={() => setFlightModalOpen(false)}
            onSave={handleSaveFlight}
          />
          <HotelsListModal
            open={hotelsModalOpen}
            onClose={() => setHotelsModalOpen(false)}
            value={hotelsValue}
            onChange={handleSaveHotels}
          />
        </>
      )}
    </div>
  );
}

export function validate(payload) {
  // Mirror the legacy InternalPassportStep validation so manager-side
  // edits still get the "must be 4 / 6 digits" guardrail. Public mode's
  // payload simply won't carry these fields, so the regex paths skip.
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
