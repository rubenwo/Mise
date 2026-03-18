export default function GenerationProgress({ events }) {
  if (events.length === 0) return null;

  return (
    <div className="generation-progress">
      <h3>Generation Progress</h3>
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
    </div>
  );
}
