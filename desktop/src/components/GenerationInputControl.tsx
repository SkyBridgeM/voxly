import { useState } from "react";
import { createCustomTemplate, type GenerationInput, type PromptTemplateSummary } from "../lib/meetingAsr";

const MAX_PROMPT_LENGTH = 20_000;

export function generationInputIsValid(input: GenerationInput) {
  return input.kind === "template"
    ? input.templateId.trim().length > 0
    : input.prompt.trim().length > 0 && input.prompt.length <= MAX_PROMPT_LENGTH;
}

export function GenerationInputControl({
  input,
  onChange,
  templates,
  locale,
  disabled = false,
}: {
  input: GenerationInput;
  onChange: (input: GenerationInput) => void;
  templates: PromptTemplateSummary[];
  locale: "zh" | "en";
  disabled?: boolean;
}) {
  const copy = locale === "zh"
    ? { template: "使用模板", temporary: "临时提示词", templateLabel: "内容模板", promptLabel: "这次想生成什么？", promptHint: "仅用于本次生成；成功后可以选择保存为模板。", placeholder: "例如：提取关键结论，并按主题整理成简洁的 Markdown。", builtIn: "内置模板", custom: "我的模板" }
    : { template: "Use template", temporary: "Temporary prompt", templateLabel: "Content template", promptLabel: "What should this generation produce?", promptHint: "Used only this time; after success, you can choose to save it as a template.", placeholder: "For example: Extract the key conclusions and organize them by topic in concise Markdown.", builtIn: "Built-in templates", custom: "My templates" };
  const firstTemplate = templates[0]?.id ?? "";

  return (
    <div className="generation-input-control">
      <div className="generation-input-tabs" role="group" aria-label={copy.promptLabel}>
        <button type="button" className={input.kind === "template" ? "is-active" : ""} onClick={() => onChange({ kind: "template", templateId: firstTemplate })} disabled={disabled}>{copy.template}</button>
        <button type="button" className={input.kind === "temporary_prompt" ? "is-active" : ""} onClick={() => onChange({ kind: "temporary_prompt", prompt: "" })} disabled={disabled}>{copy.temporary}</button>
      </div>
      {input.kind === "template" ? (
        <label className="generation-input-field">
          <span>{copy.templateLabel}</span>
          <select value={input.templateId} onChange={(event) => onChange({ kind: "template", templateId: event.target.value })} disabled={disabled}>
            <optgroup label={copy.builtIn}>{templates.filter((template) => template.built_in).map((template) => <option key={template.id} value={template.id}>{template.display_name[locale]}</option>)}</optgroup>
            {templates.some((template) => !template.built_in) && <optgroup label={copy.custom}>{templates.filter((template) => !template.built_in).map((template) => <option key={template.id} value={template.id}>{template.display_name[locale]}</option>)}</optgroup>}
          </select>
        </label>
      ) : (
        <label className="generation-input-field">
          <span>{copy.promptLabel}</span>
          <textarea value={input.prompt} maxLength={MAX_PROMPT_LENGTH} rows={5} placeholder={copy.placeholder} onChange={(event) => onChange({ kind: "temporary_prompt", prompt: event.target.value })} disabled={disabled} />
          <small>{copy.promptHint} · {input.prompt.length.toLocaleString()}/{MAX_PROMPT_LENGTH.toLocaleString()}</small>
        </label>
      )}
    </div>
  );
}

export function SaveTemporaryPromptPanel({ prompt, locale, onDismiss, onSaved }: { prompt: string; locale: "zh" | "en"; onDismiss: () => void; onSaved: () => Promise<void> | void }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState(false);
  const copy = locale === "zh"
    ? { title: "把这次提示词保存为模板？", body: "保存后可在其他任务中复用；本次生成结果不会改变。", name: "模板名称", description: "说明（可选）", cancel: "暂不保存", save: "保存为模板", saving: "正在保存…", failed: "模板保存失败，请重试。" }
    : { title: "Save this prompt as a template?", body: "Saving makes it reusable for other tasks. This generation will not change.", name: "Template name", description: "Description (optional)", cancel: "Not now", save: "Save as template", saving: "Saving…", failed: "Could not save the template. Try again." };
  const save = async () => {
    if (!name.trim() || pending) return;
    setPending(true);
    setError(false);
    try {
      await createCustomTemplate({ display_name: name.trim(), description: description.trim(), instructions: prompt });
      try { await onSaved(); } catch { /* The template is already saved; catalog refresh can retry later. */ }
      onDismiss();
    } catch {
      setError(true);
    } finally {
      setPending(false);
    }
  };
  return <section className="save-temporary-prompt" aria-label={copy.title}><div><strong>{copy.title}</strong><p>{copy.body}</p></div><label><span>{copy.name}</span><input value={name} onChange={(event) => setName(event.target.value)} maxLength={120} disabled={pending} /></label><label><span>{copy.description}</span><input value={description} onChange={(event) => setDescription(event.target.value)} maxLength={500} disabled={pending} /></label><div className="form-action-row"><button type="button" className="btn-secondary" onClick={onDismiss} disabled={pending}>{copy.cancel}</button><button type="button" className="btn-primary" onClick={() => void save()} disabled={pending || !name.trim()}>{pending ? copy.saving : copy.save}</button></div>{error && <p className="library-artifact-error" role="status">{copy.failed}</p>}</section>;
}
