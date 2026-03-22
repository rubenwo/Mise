import { useState } from 'react';
import { orderPlanAH } from '../api/client';

export default function AHOrderModal({ planId, onClose }) {
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);

  const handleSearch = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await orderPlanAH(planId);
      setResult(data);
    } catch (err) {
      setError(err.message || 'Failed to search Albert Heijn');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <h3>Order at Albert Heijn</h3>
          <button className="modal-close" onClick={onClose} aria-label="Close">&times;</button>
        </div>

        {!result && !loading && (
          <div className="modal-body">
            <p className="modal-intro">
              Search Albert Heijn for all ingredients in this plan. Ingredients that can't
              be found will be listed separately.
            </p>
            {error && <p className="modal-error">{error}</p>}
            <button className="btn btn-primary" onClick={handleSearch}>
              Search Albert Heijn
            </button>
          </div>
        )}

        {loading && (
          <div className="modal-body modal-loading">
            <div className="plan-loading-spinner" />
            <p>Searching Albert Heijn for your ingredients…</p>
          </div>
        )}

        {result && (
          <div className="modal-body">
            {result.not_found.length > 0 && (
              <div className="ah-not-found">
                <h4>Not found ({result.not_found.length})</h4>
                <p className="ah-not-found-hint">
                  These ingredients could not be found on Albert Heijn. You may need to
                  search for them manually or use a different term.
                </p>
                <ul className="ah-not-found-list">
                  {result.not_found.map((ing, i) => (
                    <li key={i}>
                      <span className="ah-not-found-name">{ing.name}</span>
                      <span className="ah-not-found-amount">{ing.amount} {ing.unit}</span>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {result.matched.length > 0 && (
              <div className="ah-matched">
                <h4>Found ({result.matched.length})</h4>
                <ul className="ah-matched-list">
                  {result.matched.map((item, i) => (
                    <li key={i} className="ah-product">
                      {item.product.image_url && (
                        <img
                          className="ah-product-img"
                          src={item.product.image_url}
                          alt={item.product.title}
                          loading="lazy"
                        />
                      )}
                      <div className="ah-product-info">
                        <span className="ah-product-title">{item.product.title}</span>
                        <span className="ah-product-sub">
                          {item.ingredient.amount} {item.ingredient.unit} {item.ingredient.name}
                          {item.product.unit_size && ` · ${item.product.unit_size}`}
                        </span>
                      </div>
                      <div className="ah-product-right">
                        {item.product.price > 0 && (
                          <span className="ah-product-price">€{item.product.price.toFixed(2)}</span>
                        )}
                        <a
                          className="btn btn-secondary btn-sm"
                          href={item.product.url}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          View
                        </a>
                      </div>
                    </li>
                  ))}
                </ul>
              </div>
            )}

            {result.matched.length === 0 && result.not_found.length === 0 && (
              <p className="empty-state">No ingredients in this plan.</p>
            )}

            <div className="modal-footer">
              <button className="btn btn-secondary" onClick={() => setResult(null)}>
                Search again
              </button>
              <button className="btn btn-primary" onClick={onClose}>
                Done
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
