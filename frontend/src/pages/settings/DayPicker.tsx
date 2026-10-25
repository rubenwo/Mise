import { WEEKDAYS, parseDays, toggleDay } from './useSettingsForm';

interface DayPickerProps {
  /** Comma-separated weekday numbers, e.g. "1,2,3". */
  value: string;
  onChange: (value: string) => void;
}

export default function DayPicker({ value, onChange }: DayPickerProps) {
  const selected = parseDays(value);
  return (
    <div className="day-picker" role="group" aria-label="Days">
      {WEEKDAYS.map(d => (
        <label key={d.value} className={`day-chip ${selected.has(d.value) ? 'day-chip-active' : ''}`}>
          <input
            type="checkbox"
            checked={selected.has(d.value)}
            onChange={() => onChange(toggleDay(value, d.value))}
          />
          {d.label}
        </label>
      ))}
    </div>
  );
}
