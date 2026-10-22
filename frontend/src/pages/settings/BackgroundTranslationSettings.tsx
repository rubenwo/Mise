import { useState } from 'react';
import { listProviders, runTranslationNow } from '../../api/client';
import { errorMessage, useFetch } from '../../hooks/useFetch';
import DayPicker from './DayPicker';
import { useSettingsForm } from './useSettingsForm';

const DEFAULTS = {
  background_translation_enabled: 'false',
  background_translation_days: '1,2,3,4,5',
  background_translation_time: '03:00',
};

export default function BackgroundTranslationSettings() {
  const providers = useFetch('providers', listProviders);
  const { values, set, save, saving, loading } = useSettingsForm(DEFAULTS);
  const [running, setRunning] = useState(false);
  const [runResult, setRunResult] = useState<string | null>(null);

  const handleRunNow = async () => {
    setRunning(true);
    setRunResult(null);
    try {
      const res = await runTranslationNow();
      setRunResult(res.message || `Translated ${res.translated} ingredient(s).`);
    } catch (err) {
      setRunResult('Error: ' + errorMessage(err));
    } finally {
      setRunning(false);
    }
  };

  if (loading || (providers.loading && !providers.data)) return null;

  const hasTranslationProvider = (providers.data ?? []).some(p => p.enabled && p.tags?.includes('translation'));
  const enabled = values.background_translation_enabled === 'true';

  return (
    <div className="settings-section">
      <h3>Background Translation</h3>
      <p className="settings-description">
        Pre-populate the translation cache on a schedule so ingredients are already translated
        when you open Albert Heijn ordering. Tag an LLM provider with <code>translation</code> to
        use a dedicated (lighter) model.
      </p>

      {!hasTranslationProvider && (
        <div className="settings-warning">
          No enabled provider with the <strong>translation</strong> tag found. Background
          translation requires at least one enabled provider tagged <code>translation</code>.
          Add the tag in the LLM Providers section above.
        </div>
      )}

      <div className="settings-form">
        <div className="settings-field">
          <label>
            <input
              type="checkbox"
              checked={enabled}
              onChange={e => set('background_translation_enabled', e.target.checked ? 'true' : 'false')}
              style={{ marginRight: 8 }}
            />
            Enable background translation
          </label>
        </div>
        {enabled && (
          <>
            <div className="settings-field">
              <label>Days</label>
              <DayPicker value={values.background_translation_days} onChange={v => set('background_translation_days', v)} />
              <span className="settings-hint">Days of the week to run translation</span>
            </div>
            <div className="settings-field">
              <label>Time of day</label>
              <input
                type="time"
                value={values.background_translation_time}
                onChange={e => set('background_translation_time', e.target.value)}
                required
              />
              <span className="settings-hint">Time to run the translation job (up to 50 ingredients per run)</span>
            </div>
          </>
        )}
        <div style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
          <button className="btn btn-primary" onClick={save} disabled={saving}>
            {saving ? 'Saving...' : 'Save'}
          </button>
          <button
            className="btn btn-secondary"
            onClick={handleRunNow}
            disabled={running || !hasTranslationProvider}
            title={!hasTranslationProvider ? 'No translation provider configured' : 'Run translation job immediately'}
          >
            {running ? 'Running…' : 'Run Now'}
          </button>
          {runResult && <span className="settings-hint">{runResult}</span>}
        </div>
      </div>
    </div>
  );
}
