import { getFeatureStatus } from '../../api/client';
import { useFetch } from '../../hooks/useFetch';

interface FeatureDef {
  tag: string;
  label: string;
  detail: string;
  downstream?: string[];
}

// Features and their human-readable implications when unavailable.
const FEATURE_DEFS: FeatureDef[] = [
  {
    tag: 'generation',
    label: 'Recipe generation',
    detail: 'Generate, import, and refine recipes; meal plan suggestions',
  },
  {
    tag: 'background-generation',
    label: 'Background generation',
    detail: 'Scheduled automatic recipe generation',
  },
  {
    tag: 'chat',
    label: 'Cooking assistant',
    detail: 'Per-recipe cooking chat',
  },
  {
    tag: 'search',
    label: 'AI recipe search',
    detail: 'Natural language search across your library',
  },
  {
    tag: 'translation',
    label: 'Ingredient translation',
    detail: 'Dutch ingredient names for Albert Heijn ordering',
    downstream: ['Dutch ingredient translation disabled', 'Order at Albert Heijn disabled'],
  },
  {
    tag: 'inventory',
    label: 'Inventory scanning',
    detail: 'Identify ingredients from photos (requires vision model)',
  },
  {
    tag: 'deduplication',
    label: 'Shopping list deduplication',
    detail: 'LLM-based ingredient deduplication for shopping lists',
  },
];

export default function FeatureStatusSection() {
  const { data, error } = useFetch('features', getFeatureStatus);
  // On failure, show every feature as unconfigured rather than hiding the section.
  const status = data ?? (error ? {} : null);

  if (!status) return null;

  const unavailable = FEATURE_DEFS.filter(f => status[f.tag] !== 'available');

  return (
    <div className="settings-section">
      <h3>Feature Status</h3>
      <p className="settings-description">
        Shows which AI features are ready. Each feature requires an enabled, healthy provider
        with the matching tag. Manage providers below.
      </p>
      <div className="feature-status-list">
        {FEATURE_DEFS.map(f => {
          const s = status[f.tag] || 'unconfigured';
          return (
            <div key={f.tag} className="feature-status-row">
              <span className={`feature-status-dot feature-status-${s}`} />
              <div className="feature-status-info">
                <span className="feature-status-label">{f.label}</span>
                <span className="feature-status-detail">{f.detail}</span>
              </div>
              <span className={`health-badge health-${s === 'available' ? 'healthy' : s === 'offline' ? 'offline' : 'unknown'}`}>
                {s}
              </span>
            </div>
          );
        })}
      </div>
      {unavailable.some(f => f.downstream) && (
        <div className="settings-warning" style={{ marginTop: 14 }}>
          {unavailable.flatMap(f => f.downstream ?? []).map(msg => (
            <div key={msg}>⚠ {msg}</div>
          ))}
        </div>
      )}
    </div>
  );
}
