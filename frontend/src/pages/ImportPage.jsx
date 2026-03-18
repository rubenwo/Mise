import { useState } from 'react';
import { saveRecipe } from '../api/client';
import GenerationProgress from '../components/GenerationProgress';
import RecipeCard from '../components/RecipeCard';
import { useGeneration } from '../hooks/useGeneration';

export default function ImportPage() {
  const { events, recipes, loading, error, generate, reset } = useGeneration();
  const [rawText, setRawText] = useState('');
  const [submittedText, setSubmittedText] = useState('');
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [feedback, setFeedback] = useState('');

  const recipe = recipes[0] || null;

  const handleImport = (e) => {
    e.preventDefault();
    if (!rawText.trim()) return;
    reset();
    setSaved(false);
    setFeedback('');
    setSubmittedText(rawText);
    generate('import', { raw_text: rawText });
  };

  const handleSave = async () => {
    if (!recipe) return;
    setSaving(true);
    try {
      await saveRecipe(recipe);
      setSaved(true);
    } catch (err) {
      alert('Failed to save: ' + err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleRefine = () => {
    if (!recipe || !feedback) return;
    setSaved(false);
    generate('refine', { recipe, feedback });
    setFeedback('');
  };

  return (
    <div className="import-page">
      <h2>Import Recipe</h2>
      <form onSubmit={handleImport} className="import-form">
        <textarea
          value={rawText}
          onChange={e => setRawText(e.target.value)}
          placeholder="Paste your recipe here — just a name is enough, but you can include ingredients and instructions too"
          rows={8}
          disabled={loading}
        />
        <button className="btn btn-primary" type="submit" disabled={loading || !rawText.trim()}>
          {loading ? 'Importing...' : 'Import Recipe'}
        </button>
      </form>

      {error && <div className="error-message">{error}</div>}
      <GenerationProgress events={events.filter(e => e.type !== 'recipe')} />

      {recipe && (
        <div className="import-comparison">
          <div className="raw-text-panel">
            <h3>Original</h3>
            <pre>{submittedText}</pre>
          </div>
          <div className="import-result-panel">
            <h3>Imported</h3>
            <RecipeCard recipe={recipe} showIngredients />
            <div className="import-actions">
              {saved ? (
                <span className="saved-badge">Saved!</span>
              ) : (
                <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
                  {saving ? 'Saving...' : 'Save Recipe'}
                </button>
              )}
              <div className="refine-section">
                <input
                  type="text"
                  placeholder="What would you like to change?"
                  value={feedback}
                  onChange={e => setFeedback(e.target.value)}
                />
                <button className="btn btn-secondary" onClick={handleRefine} disabled={loading || !feedback}>
                  Refine
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
