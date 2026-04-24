import { useEffect, useRef, useState } from 'react';
import {
  getDoverenostTemplateStatus,
  uploadDoverenostTemplate,
  getDoverenostTemplateDownloadUrl,
} from '../api/client';

function formatBytes(n) {
  if (!n) return '';
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(2)} MB`;
}

function formatDate(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  return isNaN(d) ? iso : d.toLocaleString('ru-RU');
}

export default function TemplatesPage() {
  const [status, setStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [ok, setOk] = useState(null);
  const fileRef = useRef(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const s = await getDoverenostTemplateStatus();
      setStatus(s);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const handleFileChange = async (e) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    if (!file.name.toLowerCase().endsWith('.docx')) {
      setError('Принимается только .docx');
      return;
    }
    setBusy(true);
    setError(null);
    setOk(null);
    try {
      const s = await uploadDoverenostTemplate(file);
      setStatus(s);
      setOk('Шаблон загружен');
      setTimeout(() => setOk(null), 3000);
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  };

  const isCustom = status?.custom;

  return (
    <div className="page-container">
      <div className="page-header">
        <div>
          <div className="page-title">Шаблоны документов</div>
          <div className="page-subtitle">
            Загрузите свой .docx вместо стандартного шаблона
          </div>
        </div>
      </div>

      {error && <div className="error-message">{error}</div>}
      {ok && <div className="success-message">{ok}</div>}

      {loading ? (
        <div className="loading-center"><div className="spinner spinner-lg" /></div>
      ) : (
        <div className="card">
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, marginBottom: 12 }}>
            <div>
              <div className="section-title" style={{ fontSize: 14 }}>Доверенность</div>
              <div style={{ fontSize: 11, color: 'var(--white-dim)', marginTop: 4 }}>
                Текущий файл: {isCustom
                  ? <strong style={{ color: 'var(--accent)' }}>свой шаблон</strong>
                  : <span>стандартный шаблон</span>}
                {isCustom && status.uploaded_at && (
                  <span style={{ marginLeft: 10, fontFamily: 'var(--font-mono)' }}>
                    {formatDate(status.uploaded_at)}
                    {status.size ? ` · ${formatBytes(status.size)}` : ''}
                  </span>
                )}
              </div>
            </div>
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              {isCustom && (
                <a
                  className="btn btn-secondary btn-sm"
                  href={getDoverenostTemplateDownloadUrl()}
                  target="_blank"
                  rel="noreferrer"
                  download
                >
                  Скачать текущий
                </a>
              )}
              <button
                type="button"
                className="btn btn-primary btn-sm"
                onClick={() => fileRef.current?.click()}
                disabled={busy}
              >
                {busy
                  ? <><span className="spinner" /> Загрузка…</>
                  : isCustom ? 'Заменить .docx' : 'Загрузить .docx'}
              </button>
            </div>
          </div>

          <div style={{ fontSize: 12, color: 'var(--white-dim)', lineHeight: 1.55 }}>
            Плейсхолдеры в шаблоне:
            {' '}<code>(ФИО ПО-РУССКИ)</code>,
            {' '}<code>(ДД.ММ.ГГГГ)</code>,
            {' '}<code>(СЕРИЯ НОМЕР)</code>,
            {' '}<code>(ОРГАН ВЫДАЧИ)</code>,
            {' '}<code>(АДРЕС РЕГИСТРАЦИИ)</code>,
            {' '}<code>«ДД» МЕСЯЦ ГГГГ</code>.
            <br />
            Максимальный размер файла — 5 МБ.
          </div>

          <input
            ref={fileRef}
            type="file"
            accept=".docx"
            onChange={handleFileChange}
            style={{ display: 'none' }}
          />
        </div>
      )}
    </div>
  );
}
