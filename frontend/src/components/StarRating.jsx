import { useState } from 'react';

// Convention shown in the tooltip so users remember the scale.
// 1 = never again, 6-7 = okay, 10 = amazing.
const RATING_HINT = '1 — never again · 6-7 — okay · 10 — amazing';

function ratingLabel(value) {
  if (value == null) return '';
  if (value <= 2) return 'never again';
  if (value <= 4) return 'meh';
  if (value <= 7) return 'okay';
  if (value <= 9) return 'great';
  return 'amazing';
}

// Editable 10-star rating widget. Click a star to set; click the same star
// again to clear. value is null when unrated.
export default function StarRating({ value, onChange, disabled = false }) {
  const [hover, setHover] = useState(null);
  const display = hover ?? value ?? 0;

  const handleClick = (n) => {
    if (disabled) return;
    onChange(n === value ? 0 : n); // 0 = clear
  };

  return (
    <div className="star-rating" title={RATING_HINT}>
      <div className="star-rating-stars" onMouseLeave={() => setHover(null)}>
        {Array.from({ length: 10 }, (_, i) => {
          const n = i + 1;
          const filled = n <= display;
          return (
            <button
              key={n}
              type="button"
              disabled={disabled}
              className={`star-rating-star${filled ? ' star-rating-star-filled' : ''}`}
              onMouseEnter={() => setHover(n)}
              onClick={() => handleClick(n)}
              aria-label={`Rate ${n} out of 10`}
            >
              {filled ? '★' : '☆'}
            </button>
          );
        })}
      </div>
      <div className="star-rating-label">
        {value == null
          ? <span className="star-rating-prompt">Rate this recipe</span>
          : <span><strong>{value}/10</strong> · {ratingLabel(value)}</span>}
      </div>
    </div>
  );
}

// Read-only star display. Used in CompletedPlanView and recipe history.
export function StarRatingReadOnly({ value }) {
  if (value == null) return <span className="star-rating-empty">unrated</span>;
  return (
    <span className="star-rating-readonly" title={`${value}/10 — ${ratingLabel(value)}`}>
      <span className="star-rating-readonly-stars">
        {Array.from({ length: 10 }, (_, i) => (
          <span key={i} className={i < value ? 'star-rating-star-filled' : ''}>
            {i < value ? '★' : '☆'}
          </span>
        ))}
      </span>
      <span className="star-rating-readonly-num">{value}/10</span>
    </span>
  );
}
