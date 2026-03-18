import GenerateForm from '../components/GenerateForm';
import GenerationProgress from '../components/GenerationProgress';
import ReviewPanel from '../components/ReviewPanel';
import { useGeneration } from '../hooks/useGeneration';

export default function GeneratePage() {
  const { events, recipes, loading, error, generate, reset } = useGeneration();

  const handleGenerate = (endpoint, body) => {
    reset();
    generate(endpoint, body);
  };

  const handleRefine = (recipe, feedback) => {
    generate('refine', { recipe, feedback });
  };

  return (
    <div className="generate-page">
      <h2>Generate Recipes</h2>
      <GenerateForm onGenerate={handleGenerate} loading={loading} />
      {error && <div className="error-message">{error}</div>}
      <GenerationProgress events={events.filter(e => e.type !== 'recipe')} />
      <ReviewPanel recipes={recipes} onRefine={handleRefine} loading={loading} />
    </div>
  );
}
