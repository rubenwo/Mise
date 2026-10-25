import { useSettingsForm } from './useSettingsForm';

// Display defaults only: untouched keys are not saved, so backend and
// config.yaml defaults keep applying.
const DEFAULTS = {
  suggestion_count: '3',
  default_servings: '4',
  max_tool_iterations: '5',
  generation_timeout: '120',
  ui_language: 'en',
};

export default function GeneralSettings() {
  const { values, set, save, saving, loading } = useSettingsForm(DEFAULTS, { saveDefaults: false });

  if (loading) return null;

  return (
    <div className="settings-section">
      <h3>General</h3>
      <div className="settings-form">
        <div className="settings-field">
          <label>Home picks</label>
          <input
            type="number"
            min="1"
            max="10"
            value={values.suggestion_count}
            onChange={e => set('suggestion_count', e.target.value)}
          />
          <span className="settings-hint">How many new-to-you recipes the home page suggests each day</span>
        </div>
        <div className="settings-field">
          <label>Default Servings</label>
          <input
            type="number"
            min="1"
            max="20"
            value={values.default_servings}
            onChange={e => set('default_servings', e.target.value)}
          />
          <span className="settings-hint">Used as default when generating recipes or creating meal plans</span>
        </div>
        <div className="settings-field">
          <label>Max Tool Iterations</label>
          <input
            type="number"
            min="1"
            max="20"
            value={values.max_tool_iterations}
            onChange={e => set('max_tool_iterations', e.target.value)}
          />
          <span className="settings-hint">Maximum number of tool-use rounds per generation</span>
        </div>
        <div className="settings-field">
          <label>Generation Timeout (seconds)</label>
          <input
            type="number"
            min="10"
            max="600"
            value={values.generation_timeout}
            onChange={e => set('generation_timeout', e.target.value)}
          />
          <span className="settings-hint">HTTP timeout for each Ollama request</span>
        </div>
        <div className="settings-field">
          <label>UI Language</label>
          <select
            value={values.ui_language}
            onChange={e => set('ui_language', e.target.value)}
          >
            <option value="en">English</option>
            <option value="nl">Dutch (Nederlands)</option>
          </select>
          <span className="settings-hint">
            Display language for the interface. Albert Heijn searches always use Dutch regardless of this setting.
            To use a dedicated model for translation, tag an Ollama provider with the <code>translation</code> tag.
          </span>
        </div>
        <button className="btn btn-primary" onClick={save} disabled={saving}>
          {saving ? 'Saving...' : 'Save Settings'}
        </button>
      </div>
    </div>
  );
}
