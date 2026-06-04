import { useState, useRef, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';
import RecipeCard from './RecipeCard';
import { recommendRecipes } from '../api/client';

const newMsgId = () =>
  (typeof crypto !== 'undefined' && crypto.randomUUID)
    ? crypto.randomUUID()
    : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;

export default function LibraryRecommendChat({ limit, onDeleteRecipe }) {
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const bottomRef = useRef(null);
  const shouldScrollRef = useRef(false);

  useEffect(() => {
    if (shouldScrollRef.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
      shouldScrollRef.current = false;
    }
  }, [messages, loading]);

  const handleSend = async (e) => {
    e.preventDefault();
    const text = input.trim();
    if (!text || loading) return;

    const userMsg = { id: newMsgId(), role: 'user', content: text };
    const nextHistory = [...messages, userMsg];
    setMessages(nextHistory);
    setInput('');
    setLoading(true);
    shouldScrollRef.current = true;

    try {
      const payload = nextHistory.map(m => ({ role: m.role, content: m.content }));
      const result = await recommendRecipes(payload, limit);
      setMessages(prev => [
        ...prev,
        {
          id: newMsgId(),
          role: 'assistant',
          content: result.message || '',
          recipes: result.recipes || [],
        },
      ]);
    } catch (err) {
      setMessages(prev => [
        ...prev,
        {
          id: newMsgId(),
          role: 'assistant',
          content: `Something went wrong: ${err.message}`,
          recipes: [],
        },
      ]);
    } finally {
      setLoading(false);
    }
  };

  const handleReset = () => {
    setMessages([]);
    setInput('');
  };

  return (
    <div className="library-recommend-chat">
      <div className="library-recommend-chat-header">
        <p className="library-recommend-hint">
          Describe what you're looking for — group size, dietary needs, ingredients to avoid — and I'll pick recipes from your library.
        </p>
        {messages.length > 0 && (
          <button type="button" className="btn btn-secondary btn-sm" onClick={handleReset} disabled={loading}>
            New chat
          </button>
        )}
      </div>

      <div className="chat-messages library-recommend-messages">
        {messages.length === 0 && (
          <p className="chat-empty">
            Try: "I'm cooking for 8 vegetarians, balanced and not too exotic" or "something quick with chicken under 30 minutes".
          </p>
        )}
        {messages.map((msg) => (
          <div key={msg.id} className={`chat-message chat-message-${msg.role}`}>
            <span className="chat-role">{msg.role === 'user' ? 'You' : 'Mise'}</span>
            <div className="chat-bubble">
              {msg.role === 'assistant'
                ? <ReactMarkdown>{msg.content}</ReactMarkdown>
                : <p>{msg.content}</p>}
            </div>
            {msg.role === 'assistant' && msg.recipes && msg.recipes.length > 0 && (
              <div className="library-recommend-results">
                {msg.recipes.map(recipe => (
                  <RecipeCard
                    key={recipe.id}
                    recipe={recipe}
                    showLink
                    onDelete={onDeleteRecipe}
                  />
                ))}
              </div>
            )}
          </div>
        ))}
        {loading && (
          <div className="chat-message chat-message-assistant">
            <span className="chat-role">Mise</span>
            <div className="chat-bubble">
              <p className="chat-thinking">Looking through your library…</p>
            </div>
          </div>
        )}
        <div ref={bottomRef} />
      </div>

      <form className="chat-input-form" onSubmit={handleSend}>
        <input
          type="text"
          value={input}
          onChange={e => setInput(e.target.value)}
          placeholder="Describe what you're looking for…"
          disabled={loading}
          className="chat-input"
        />
        <button type="submit" className="btn btn-primary" disabled={loading || !input.trim()}>
          {loading ? '…' : 'Ask'}
        </button>
      </form>
    </div>
  );
}
