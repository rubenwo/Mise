import { Minus, Plus } from 'lucide-react';

interface StepperProps {
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
  /** Renders the value, e.g. n => `${n} servings`. */
  format?: (value: number) => string;
  label: string;
}

/** A compact −/+ control for small whole numbers such as servings. */
export default function Stepper({ value, onChange, min = 1, max = 20, format = String, label }: StepperProps) {
  return (
    <div className="stepper" role="group" aria-label={label}>
      <button type="button" aria-label={`Fewer ${label.toLowerCase()}`} disabled={value <= min} onClick={() => onChange(value - 1)}>
        <Minus size={16} aria-hidden="true" />
      </button>
      <span className="stepper-value" aria-live="polite">{format(value)}</span>
      <button type="button" aria-label={`More ${label.toLowerCase()}`} disabled={value >= max} onClick={() => onChange(value + 1)}>
        <Plus size={16} aria-hidden="true" />
      </button>
    </div>
  );
}
