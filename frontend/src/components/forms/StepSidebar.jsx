// StepSidebar — left rail listing the 7 wizard steps.
//   - current step → highlighted (accent border + bold)
//   - past steps → marked complete with a check; clickable to jump back
//   - future steps → visually disabled, NOT clickable
// Also renders a thin progress bar with "X из 7" label below the list.

export default function StepSidebar({ steps, currentStep, onJump }) {
  const total = steps.length;
  const progress = Math.round(((currentStep + 1) / total) * 100);

  return (
    <aside className="fw-sidebar" aria-label="Шаги анкеты">
      <div className="fw-sidebar-title">Анкета</div>

      {steps.map((step, i) => {
        const isActive = i === currentStep;
        const isDone = i < currentStep;
        const isFuture = i > currentStep;
        const clickable = isDone; // only past steps are clickable
        const cls = [
          'fw-step',
          isActive ? 'is-active' : '',
          isDone ? 'is-done' : '',
          isFuture ? 'is-future' : '',
          clickable ? 'is-clickable' : '',
        ].filter(Boolean).join(' ');

        // Marker shows the step number normally, a check (✓) when done.
        const marker = isDone ? '✓' : String(i + 1);

        return (
          <button
            type="button"
            key={step.id}
            className={cls}
            onClick={() => clickable && onJump(i)}
            disabled={!clickable && !isActive}
            aria-current={isActive ? 'step' : undefined}
          >
            <span className="fw-step-marker" aria-hidden="true">{marker}</span>
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
