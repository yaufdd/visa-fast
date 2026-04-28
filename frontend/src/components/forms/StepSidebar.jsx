// StepSidebar — left rail listing the 7 wizard steps. Every step is
// clickable; the user can jump to any section at any time. Sections
// look identical except for the currently-active one.

export default function StepSidebar({ steps, currentStep, onJump }) {
  const total = steps.length;
  const progress = Math.round(((currentStep + 1) / total) * 100);

  return (
    <aside className="fw-sidebar" aria-label="Шаги анкеты">
      <div className="fw-sidebar-title">Анкета</div>

      {steps.map((step, i) => {
        const isActive = i === currentStep;
        const clickable = !isActive;
        const cls = [
          'fw-step',
          isActive ? 'is-active' : '',
          clickable ? 'is-clickable' : '',
        ].filter(Boolean).join(' ');

        return (
          <button
            type="button"
            key={step.id}
            className={cls}
            onClick={() => clickable && onJump(i)}
            aria-current={isActive ? 'step' : undefined}
          >
            <span className="fw-step-marker" aria-hidden="true">{i + 1}</span>
            <span className="fw-step-label">{step.label}</span>
          </button>
        );
      })}

      <div className="fw-progress-meter">
        <div className="fw-progress-bar">
          <div className="fw-progress-fill" style={{ width: `${progress}%` }} />
        </div>
        <div className="fw-progress-label">Шаг {currentStep + 1} из {total}</div>
      </div>
    </aside>
  );
}
