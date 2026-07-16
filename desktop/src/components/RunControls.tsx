import type { OutputMode, PromptTemplateSummary } from "../lib/meetingAsr";

interface RunControlsProps {
  outputMode: OutputMode;
  onModeChange: (mode: OutputMode) => void;
  templates: PromptTemplateSummary[];
  templateId: string;
  onTemplateChange: (templateId: string) => void;
  locale: "zh" | "en";
  onStartRun: () => void;
  isRunning: boolean;
  audioPath: string | null;
  labels: {
    modeTranscript: string;
    modeTranscriptDesc: string;
    modeGenerate: string;
    modeGenerateDesc: string;
    templateLabel: string;
    start: string;
    processing: string;
    llmWarning: string;
    transcriptWarning: string;
  };
}

export function RunControls({ outputMode, onModeChange, templates, templateId, onTemplateChange, locale, onStartRun, isRunning, audioPath, labels }: RunControlsProps) {
  return (
    <>
      <div className="mode-options">
        <div className="mode-option">
          <input type="radio" id="mode-transcript" name="mode" value="transcript_only" checked={outputMode === "transcript_only"} onChange={() => onModeChange("transcript_only")} disabled={isRunning} />
          <label htmlFor="mode-transcript"><span className="mode-index">01</span><span className="mode-copy"><strong>{labels.modeTranscript}</strong><small>{labels.modeTranscriptDesc}</small></span><span className="mode-selected" aria-hidden="true">✓</span></label>
        </div>
        <div className="mode-option">
          <input type="radio" id="mode-generate" name="mode" value="transcript_and_generate" checked={outputMode === "transcript_and_generate"} onChange={() => onModeChange("transcript_and_generate")} disabled={isRunning} />
          <label htmlFor="mode-generate"><span className="mode-index">02</span><span className="mode-copy"><strong>{labels.modeGenerate}</strong><small>{labels.modeGenerateDesc}</small></span><span className="mode-selected" aria-hidden="true">✓</span></label>
        </div>
      </div>
      {outputMode === "transcript_and_generate" && (
        <div className="template-selector">
          <label htmlFor="generation-template">{labels.templateLabel}</label>
          <select id="generation-template" value={templateId} onChange={(event) => onTemplateChange(event.target.value)} disabled={isRunning}>
            {templates.map((template) => <option key={template.id} value={template.id}>{template.display_name[locale]}</option>)}
          </select>
          {templates.find((template) => template.id === templateId) && <p>{templates.find((template) => template.id === templateId)!.description[locale]}</p>}
        </div>
      )}
      <div className="warning-banner"><span className="warning-icon" aria-hidden="true">i</span><span>{outputMode === "transcript_and_generate" ? labels.llmWarning : labels.transcriptWarning}</span></div>
      <div className="section run-section"><button className="btn-primary" onClick={onStartRun} disabled={!audioPath || isRunning || (outputMode === "transcript_and_generate" && !templateId)}>{isRunning ? <><span className="spinner"></span>{labels.processing}</> : <>{labels.start}<span className="button-arrow" aria-hidden="true">→</span></>}</button></div>
    </>
  );
}
