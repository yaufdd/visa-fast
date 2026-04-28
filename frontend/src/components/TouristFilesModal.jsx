import Modal from './Modal';
import SubmissionFilesPanel from './SubmissionFilesPanel';

// Thin wrapper that pops open the SubmissionFilesPanel for a single
// tourist. Used by the file-count badge on TouristCard.
export default function TouristFilesModal({
  open, onClose, submissionId, touristId, touristName, onUpdated, onParseRequest,
}) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`Документы — ${touristName || 'турист'}`}
      width={620}
    >
      {submissionId ? (
        <SubmissionFilesPanel
          submissionId={submissionId}
          touristId={touristId}
          onUpdated={onUpdated}
          onParseRequest={onParseRequest}
        />
      ) : (
        <div style={{ color: 'var(--white-dim)', fontSize: 13 }}>
          У туриста нет привязанной заявки.
        </div>
      )}
    </Modal>
  );
}
