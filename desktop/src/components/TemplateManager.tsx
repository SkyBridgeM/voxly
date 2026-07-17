import { useState } from "react";
import {
  createCustomTemplate,
  deleteCustomTemplate,
  updateCustomTemplate,
  type CustomTemplateInput,
  type PromptTemplateSummary,
} from "../lib/meetingAsr";

interface Labels {
  builtInTemplates: string;
  myTemplates: string;
  newTemplate: string;
  editTemplate: string;
  deleteTemplate: string;
  templateName: string;
  templateDescription: string;
  templateInstructions: string;
  revision: string;
  saveCreatesRevision: string;
  confirmDeleteTemplate: string;
  templateSaveFailed: string;
  noCustomTemplates: string;
  save: string;
  cancel: string;
}

const EMPTY_INPUT: CustomTemplateInput = { display_name: "", description: "", instructions: "" };

export function TemplateManager({ templates, locale, labels, disabled, onChanged }: { templates: PromptTemplateSummary[]; locale: "zh" | "en"; labels: Labels; disabled: boolean; onChanged: () => Promise<void> }) {
  const [editing, setEditing] = useState<PromptTemplateSummary | "new" | null>(null);
  const [draft, setDraft] = useState<CustomTemplateInput>(EMPTY_INPUT);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const builtins = templates.filter((template) => template.built_in);
  const custom = templates.filter((template) => !template.built_in);

  const beginCreate = () => { setEditing("new"); setDraft(EMPTY_INPUT); setError(null); };
  const beginEdit = (template: PromptTemplateSummary) => { setEditing(template); setDraft({ display_name: template.display_name.en, description: template.description.en, instructions: template.instructions }); setError(null); };
  const cancel = () => { setEditing(null); setDraft(EMPTY_INPUT); setError(null); };
  const save = async () => {
    if (!draft.display_name.trim() || !draft.instructions.trim()) return;
    setPending(true); setError(null);
    try {
      if (editing === "new") await createCustomTemplate(draft);
      else if (editing) await updateCustomTemplate({ ...draft, template_id: editing.id, expected_current_revision: editing.revision });
      await onChanged(); cancel();
    } catch { setError(labels.templateSaveFailed); }
    finally { setPending(false); }
  };
  const remove = async (template: PromptTemplateSummary) => {
    if (!window.confirm(labels.confirmDeleteTemplate)) return;
    setPending(true); setError(null);
    try { await deleteCustomTemplate(template.id, template.revision); await onChanged(); if (editing !== "new" && editing?.id === template.id) cancel(); }
    catch { setError(labels.templateSaveFailed); }
    finally { setPending(false); }
  };

  return <div className="template-manager">
    <div className="template-manager-group"><h4>{labels.builtInTemplates}</h4><div className="template-manager-list">{builtins.map((template) => <div className="template-manager-item readonly" key={template.id}><div><strong>{template.display_name[locale]}</strong><p>{template.description[locale]}</p></div><span>{labels.revision} {template.revision}</span></div>)}</div></div>
    <div className="template-manager-group"><div className="template-manager-heading"><h4>{labels.myTemplates}</h4><button className="btn-secondary" onClick={beginCreate} disabled={disabled || pending}>{labels.newTemplate}</button></div>{custom.length === 0 ? <p className="template-manager-empty">{labels.noCustomTemplates}</p> : <div className="template-manager-list">{custom.map((template) => <div className="template-manager-item" key={template.id}><div><strong>{template.display_name.en}</strong><p>{template.description.en}</p><span>{labels.revision} {template.revision}</span></div><div><button className="btn-secondary" onClick={() => beginEdit(template)} disabled={disabled || pending}>{labels.editTemplate}</button><button className="btn-secondary" onClick={() => void remove(template)} disabled={disabled || pending}>{labels.deleteTemplate}</button></div></div>)}</div>}</div>
    {editing && <form className="template-editor" onSubmit={(event) => { event.preventDefault(); void save(); }}><h4>{editing === "new" ? labels.newTemplate : labels.editTemplate}</h4><label><span>{labels.templateName}</span><input value={draft.display_name} onChange={(event) => setDraft((current) => ({...current, display_name: event.target.value}))} required maxLength={120} disabled={pending} /></label><label><span>{labels.templateDescription}</span><textarea value={draft.description} onChange={(event) => setDraft((current) => ({...current, description: event.target.value}))} maxLength={1000} disabled={pending} /></label><label><span>{labels.templateInstructions}</span><textarea className="template-instructions" value={draft.instructions} onChange={(event) => setDraft((current) => ({...current, instructions: event.target.value}))} required maxLength={20000} disabled={pending} /></label>{editing !== "new" && <p>{labels.saveCreatesRevision}</p>}<div className="template-editor-actions"><button type="button" className="btn-secondary" onClick={cancel} disabled={pending}>{labels.cancel}</button><button type="submit" className="btn-primary" disabled={pending || !draft.display_name.trim() || !draft.instructions.trim()}>{labels.save}</button></div></form>}
    {error && <p className="template-manager-error" role="status">{error}</p>}
  </div>;
}
