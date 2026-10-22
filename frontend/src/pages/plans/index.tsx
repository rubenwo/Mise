import { useParams } from 'react-router-dom';
import PlanDetail from './PlanDetail';
import PlansList from './PlansList';

export default function PlanPage() {
  const { id } = useParams();
  // key resets all per-plan state when navigating between plans.
  if (id) return <PlanDetail key={id} planId={parseInt(id, 10)} />;
  return <PlansList />;
}
