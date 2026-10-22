import DayPicker from './DayPicker';
import { useSettingsForm } from './useSettingsForm';

const DEFAULTS = {
  background_generation_enabled: 'false',
  background_generation_days: '1,2,3,4,5',
  background_generation_time: '08:00',
  background_generation_count: '1',
  background_generation_max_retries: '3',
};

export default function BackgroundGenerationSettings() {
  const { values, set, save, saving, loading } = useSettingsForm(DEFAULTS);

  if (loading) return null;

  const enabled = values.background_generation_enabled === 'true';

  return (
    <div className="settings-section">
      <h3>Background Generation</h3>
      <p className="settings-description">
        Automatically generate recipes on a cron-like schedule. Tag a provider with <code>background-generation</code>
        (e.g. your always-on server) to dedicate it to this task.
      </p>
      <div className="settings-form">
        <div className="settings-field">
          <label>
            <input
              type="checkbox"
              checked={enabled}
              onChange={e => set('background_generation_enabled', e.target.checked ? 'true' : 'false')}
              style={{ marginRight: 8 }}
            />
            Enable background generation
          </label>
        </div>
        {enabled && (
          <>
            <div className="settings-field">
              <label>Days</label>
              <DayPicker value={values.background_generation_days} onChange={v => set('background_generation_days', v)} />
              <span className="settings-hint">Days of the week to run generation</span>
            </div>
            <div className="settings-field">
              <label>Time of day</label>
              <input
                type="time"
                value={values.background_generation_time}
                onChange={e => set('background_generation_time', e.target.value)}
                required
              />
              <span className="settings-hint">Time to run generation on selected days</span>
            </div>
            <div className="settings-field">
              <label>Recipes per run</label>
              <input
                type="number"
                min="1"
                max="10"
                value={values.background_generation_count}
                onChange={e => set('background_generation_count', e.target.value)}
              />
              <span className="settings-hint">Number of recipes generated each run</span>
            </div>
            <div className="settings-field">
              <label>Max retries on invalid JSON</label>
              <input
                type="number"
                min="0"
                max="10"
                value={values.background_generation_max_retries}
                onChange={e => set('background_generation_max_retries', e.target.value)}
              />
              <span className="settings-hint">How many times to retry a recipe if the model returns invalid JSON (0 = no retries)</span>
            </div>
          </>
        )}
        <button className="btn btn-primary" onClick={save} disabled={saving}>
          {saving ? 'Saving...' : 'Save'}
        </button>
      </div>
    </div>
  );
}
