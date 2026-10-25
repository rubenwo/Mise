import { useState, type ChangeEvent, type FormEvent } from 'react';
import { createProvider, deleteProvider, listModels, listProviders, updateProvider } from '../../api/client';
import { errorMessage, useFetch } from '../../hooks/useFetch';
import type { Provider, ProviderInput, ProviderType } from '../../types';

const KNOWN_TAGS = ['generation', 'background-generation', 'chat', 'search', 'translation', 'inventory', 'deduplication', 'embedding'];

const EMPTY_FORM: Omit<ProviderInput, 'enabled'> = { name: '', host: '', model: '', provider_type: 'ollama', tags: [] };

function TagPicker({ tags = [], onChange }: { tags?: string[]; onChange: (tags: string[]) => void }) {
  const [selected, setSelected] = useState('');

  const add = () => {
    if (!selected || tags.includes(selected)) return;
    onChange([...tags, selected]);
    setSelected('');
  };

  const remove = (tag: string) => onChange(tags.filter(t => t !== tag));

  const availableKnown = KNOWN_TAGS.filter(t => !tags.includes(t));

  return (
    <div className="tag-picker">
      {tags.length > 0 && (
        <div className="tag-picker-chips">
          {tags.map(t => (
            <span key={t} className="tag tag-chip">
              {t}
              <button type="button" className="tag-chip-remove" onClick={() => remove(t)} aria-label={`Remove ${t}`}>
                &times;
              </button>
            </span>
          ))}
        </div>
      )}
      <div className="tag-picker-input-row">
        <select
          value={selected}
          onChange={e => setSelected(e.target.value)}
        >
          <option value="">Add tag…</option>
          {availableKnown.map(t => <option key={t} value={t}>{t}</option>)}
        </select>
        <button
          type="button"
          className="btn btn-secondary"
          onClick={add}
          disabled={!selected}
        >
          Add
        </button>
      </div>
    </div>
  );
}

interface ModelInputProps {
  host: string;
  providerType?: ProviderType;
  value: string;
  onChange: (e: ChangeEvent<HTMLInputElement>) => void;
  id: string;
}

/** Model name input with suggestions fetched from the provider's host. */
function ModelInput({ host, providerType = 'ollama', value, onChange, id }: ModelInputProps) {
  const { data: models } = useFetch(
    host ? `models:${providerType}:${host}` : null,
    signal => listModels(host, providerType, signal),
  );

  return (
    <>
      <input
        list={id}
        value={value}
        onChange={onChange}
        placeholder="Model (e.g. qwen3.5:9b)"
      />
      <datalist id={id}>
        {(models ?? []).map(m => <option key={m} value={m} />)}
      </datalist>
    </>
  );
}

export default function ProvidersSection() {
  const { data, loading, reload } = useFetch('providers', listProviders);
  const providers = data ?? [];
  const [form, setForm] = useState(EMPTY_FORM);
  const [editing, setEditing] = useState<Provider | null>(null);

  const run = async (action: () => Promise<unknown>) => {
    try {
      await action();
      reload();
    } catch (err) {
      alert('Provider update failed: ' + errorMessage(err));
    }
  };

  const handleAdd = (e: FormEvent) => {
    e.preventDefault();
    if (!form.name || !form.host || !form.model) return;
    run(async () => {
      await createProvider({ ...form, enabled: true });
      setForm(EMPTY_FORM);
    });
  };

  const handleDelete = (id: number) => run(() => deleteProvider(id));

  const handleToggle = (p: Provider) => run(() => updateProvider(p.id, { ...p, enabled: !p.enabled }));

  const handleEditSave = () => {
    if (!editing) return;
    run(async () => {
      await updateProvider(editing.id, editing);
      setEditing(null);
    });
  };

  if (loading && !data) return <p className="empty-state">Loading...</p>;

  return (
    <div className="settings-section">
      <h3>LLM Providers</h3>
      <p className="settings-description">
        Configure LLM endpoints. Supports native Ollama and any OpenAI-compatible API (LiteLLM, vLLM, llama.cpp).
        Batch generation distributes work across enabled providers.
      </p>

      <div className="providers-list">
        {providers.map(p => (
          <div key={p.id} className={`provider-item ${!p.enabled ? 'provider-disabled' : ''}`}>
            {editing?.id === p.id ? (
              <div className="provider-edit-form">
                <input
                  value={editing.name}
                  onChange={e => setEditing(prev => prev && { ...prev, name: e.target.value })}
                  placeholder="Name"
                />
                <input
                  value={editing.host}
                  onChange={e => setEditing(prev => prev && { ...prev, host: e.target.value })}
                  placeholder="Host URL"
                />
                <select
                  value={editing.provider_type || 'ollama'}
                  onChange={e => setEditing(prev => prev && { ...prev, provider_type: e.target.value as ProviderType })}
                >
                  <option value="ollama">Ollama</option>
                  <option value="openai_compat">OpenAI-compatible</option>
                </select>
                <ModelInput
                  host={editing.host}
                  providerType={editing.provider_type || 'ollama'}
                  value={editing.model}
                  onChange={e => setEditing(prev => prev && { ...prev, model: e.target.value })}
                  id={`edit-models-${editing.id}`}
                />
                <TagPicker
                  tags={editing.tags || []}
                  onChange={tags => setEditing(prev => prev && { ...prev, tags })}
                />
                <div className="provider-edit-actions">
                  <button className="btn btn-primary" onClick={handleEditSave}>Save</button>
                  <button className="btn btn-secondary" onClick={() => setEditing(null)}>Cancel</button>
                </div>
              </div>
            ) : (
              <>
                <div className="provider-info">
                  <strong>{p.name}</strong>
                  <span className="provider-host">{p.host}</span>
                  <span className="provider-model">{p.model}</span>
                  {p.provider_type && p.provider_type !== 'ollama' && (
                    <span className="tag">{p.provider_type}</span>
                  )}
                  {p.tags && p.tags.length > 0 && (
                    <span className="provider-tags">
                      {p.tags.map(t => <span key={t} className="tag">{t}</span>)}
                    </span>
                  )}
                  <span className={`health-badge health-${p.health_status}`}>{p.health_status}</span>
                </div>
                <div className="provider-actions">
                  <button
                    className={`btn ${p.enabled ? 'btn-secondary' : 'btn-primary'}`}
                    onClick={() => handleToggle(p)}
                  >
                    {p.enabled ? 'Disable' : 'Enable'}
                  </button>
                  <button className="btn btn-secondary" onClick={() => setEditing({ ...p })}>Edit</button>
                  <button className="btn btn-danger" onClick={() => handleDelete(p.id)}>Delete</button>
                </div>
              </>
            )}
          </div>
        ))}
        {providers.length === 0 && (
          <p className="empty-state">No providers configured.</p>
        )}
      </div>

      <form onSubmit={handleAdd} className="provider-add-form">
        <h4>Add Provider</h4>
        <div className="provider-add-fields">
          <input
            value={form.name}
            onChange={e => setForm({ ...form, name: e.target.value })}
            placeholder="Name (e.g. GPU Server)"
          />
          <input
            value={form.host}
            onChange={e => setForm({ ...form, host: e.target.value })}
            placeholder="Host URL (e.g. http://192.168.1.100:11434)"
          />
          <select
            value={form.provider_type}
            onChange={e => setForm({ ...form, provider_type: e.target.value as ProviderType })}
          >
            <option value="ollama">Ollama</option>
            <option value="openai_compat">OpenAI-compatible</option>
          </select>
          <ModelInput
            host={form.host}
            providerType={form.provider_type}
            value={form.model}
            onChange={e => setForm({ ...form, model: e.target.value })}
            id="add-models"
          />
          <TagPicker
            tags={form.tags}
            onChange={tags => setForm(f => ({ ...f, tags }))}
          />
        </div>
        <button
          className="btn btn-primary"
          type="submit"
          disabled={!form.name || !form.host || !form.model}
        >
          Add Provider
        </button>
      </form>
    </div>
  );
}
