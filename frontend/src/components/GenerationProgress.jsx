import { useState, useEffect } from 'react';

export default function GenerationProgress({ events, loading, hasRecipes }) {
  const [collapsed, setCollapsed] = useState(false);

  // Auto-expand when generation starts, auto-collapse when recipes arrive
  useEffect(() => {
    if (loading) setCollapsed(false);
  }, [loading]);

  useEffect(() => {
    if (hasRecipes && !loading) setCollapsed(true);
  }, [hasRecipes, loading]);

  if (events.length === 0) return null;

  const lastEvent = events[events.length - 1];
  const isActive = lastEvent.type !== 'error' && loading;

  return (
    <div className={`generation-progress ${collapsed ? 'collapsed' : ''}`}>
      <button type="button" className="progress-header" onClick={() => setCollapsed(c => !c)}>
        <h3>Generation Progress</h3>
        {isActive && <span className="loading-dots"><span /><span /><span /></span>}
        <span className="progress-toggle">{collapsed ? '\u25B6' : '\u25BC'}</span>
      </button>
      {!collapsed && (
        <div className="events-list">
          {events.map((event, i) => (
            <div key={i} className={`event event-${event.type}`}>
              {event.type === 'status' && <span className="event-status">{event.message}</span>}
              {event.type === 'tool_call' && (
                <span className="event-tool">Calling tool: <strong>{event.tool}</strong></span>
              )}
              {event.type === 'tool_result' && (
                <span className="event-result">Got results from: <strong>{event.tool}</strong></span>
              )}
              {event.type === 'error' && (
                <span className="event-error">Error: {event.message}</span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
