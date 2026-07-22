import type { GenerationInput, OutputMode, PromptTemplateSummary } from "../lib/meetingAsr";
import { ArrowRight, Check } from "@phosphor-icons/react";
import { GenerationInputControl, generationInputIsValid } from "./GenerationInputControl";

interface RunControlsProps {
  outputMode: OutputMode;
  onModeChange: (mode: OutputMode) => void;
  templates: PromptTemplateSummary[];
  generationInput: GenerationInput;
  onGenerationInputChange: (input: GenerationInput) => void;
  locale: "zh" | "en";
  onStartRun: () => void;
  isRunning: boolean;
  audioPath: string | null;
  ready: boolean;
  asrSummary: string;
  llmSummary: string;
  onOpenSettings: () => void;
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
    speechRecognitionProvider: string;
    generatedWithProvider: string;
    providerNotReady: string;
    openSettings: string;
    builtInTemplates: string;
    myTemplates: string;
  };
}

export function RunControls({ outputMode, onModeChange, templates, generationInput, onGenerationInputChange, locale, onStartRun, isRunning, audioPath, ready, asrSummary, llmSummary, onOpenSettings, labels }: RunControlsProps) {
  return (
    <>
      <div className="mode-options">
        <div className="mode-option">
          <input type="radio" id="mode-transcript" name="mode" value="transcript_only" checked={outputMode === "transcript_only"} onChange={() => onModeChange("transcript_only")} disabled={isRunning} />
          <label htmlFor="mode-transcript"><span className="mode-index">01</span><span className="mode-copy"><strong>{labels.modeTranscript}</strong><small>{labels.modeTranscriptDesc}</small></span><span className="mode-selected" aria-hidden="true"><Check /></span></label>
        </div>
        <div className="mode-option">
          <input type="radio" id="mode-generate" name="mode" value="transcript_and_generate" checked={outputMode === "transcript_and_generate"} onChange={() => onModeChange("transcript_and_generate")} disabled={isRunning} />
          <label htmlFor="mode-generate"><span className="mode-index">02</span><span className="mode-copy"><strong>{labels.modeGenerate}</strong><small>{labels.modeGenerateDesc}</small></span><span className="mode-selected" aria-hidden="true"><Check /></span></label>
        </div>
      </div>
      {outputMode === "transcript_and_generate" && (
        <div className="template-selector">
          <GenerationInputControl input={generationInput} onChange={onGenerationInputChange} templates={templates} locale={locale} disabled={isRunning} />
          {generationInput.kind === "template" && templates.find((template) => template.id === generationInput.templateId) && <p>{templates.find((template) => template.id === generationInput.templateId)!.description[locale]}</p>}
        </div>
      )}
      <div className="provider-run-summary"><span>{labels.speechRecognitionProvider}: <strong>{asrSummary}</strong></span>{outputMode === "transcript_and_generate" && <span>{labels.generatedWithProvider}: <strong>{llmSummary}</strong></span>}{!ready && <span className="provider-not-ready">{labels.providerNotReady} <button className="link-button" onClick={onOpenSettings}>{labels.openSettings}</button></span>}</div>
      <div className="warning-banner"><span className="warning-icon" aria-hidden="true">i</span><span>{outputMode === "transcript_and_generate" ? labels.llmWarning : labels.transcriptWarning}</span></div>
      <div className="section run-section"><button className="btn-primary" onClick={onStartRun} disabled={!audioPath || isRunning || !ready || (outputMode === "transcript_and_generate" && !generationInputIsValid(generationInput))}>{isRunning ? <><span className="spinner"></span>{labels.processing}</> : <>{labels.start}<ArrowRight className="button-arrow" aria-hidden="true" /></>}</button></div>
    </>
  );
}
