import { useState, useRef, useCallback, type FormEvent } from 'react';
import { listInventory, createInventoryItem, updateInventoryItem, deleteInventoryItem,
         scanIngredient, listPendingScans, deletePendingScan } from '../api/client';
import { errorMessage, useFetch } from '../hooks/useFetch';
import { revalidateInventory } from '../hooks/useInventory';
import type { InventoryInput, InventoryItem, PendingIngredientScan } from '../types';

// Amount is a string while editing so the field can be empty.
type ItemDraft = Omit<InventoryInput, 'amount'> & { amount: string };

const EMPTY_FORM: ItemDraft = { name: '', amount: '', unit: '', notes: '' };
let nextLocalId = 1; // local-only IDs for in-flight placeholders

const byName = (a: InventoryItem, b: InventoryItem) => a.name.localeCompare(b.name);

interface ItemFormProps {
  initial?: ItemDraft;
  onSave: (item: InventoryInput) => void;
  onCancel?: () => void;
  saving: boolean;
}

function ItemForm({ initial = EMPTY_FORM, onSave, onCancel, saving }: ItemFormProps) {
  const [form, setForm] = useState(initial);
  const set = (f: keyof ItemDraft, v: string) => setForm(prev => ({ ...prev, [f]: v }));

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!form.name.trim()) return;
    onSave({
      name: form.name.trim(),
      amount: parseFloat(form.amount) || 0,
      unit: form.unit.trim(),
      notes: form.notes.trim(),
    });
  };

  return (
    <form className="inventory-item-form" onSubmit={handleSubmit}>
      <div className="inventory-form-row">
        <div className="form-group" style={{ flex: 2 }}>
          <label>Name *</label>
          <input className="edit-input" value={form.name} onChange={e => set('name', e.target.value)}
            placeholder="e.g. whole milk" autoFocus />
        </div>
        <div className="form-group" style={{ flex: 1 }}>
          <label>Amount</label>
          <input className="edit-input" type="number" min="0" step="any" value={form.amount}
            onChange={e => set('amount', e.target.value)} placeholder="0" />
        </div>
        <div className="form-group" style={{ flex: 1 }}>
          <label>Unit</label>
          <input className="edit-input" value={form.unit} onChange={e => set('unit', e.target.value)}
            placeholder="g, ml, pcs…" />
        </div>
      </div>
      <div className="form-group">
        <label>Notes</label>
        <input className="edit-input" value={form.notes} onChange={e => set('notes', e.target.value)}
          placeholder="optional" />
      </div>
      <div className="inventory-form-actions">
        <button className="btn btn-primary" type="submit" disabled={saving || !form.name.trim()}>
          {saving ? 'Saving…' : 'Save'}
        </button>
        {onCancel && <button className="btn btn-secondary" type="button" onClick={onCancel}>Cancel</button>}
      </div>
    </form>
  );
}

function ScanPanel({ onQueued }: { onQueued: (file: File) => void }) {
  const inputRef = useRef<HTMLInputElement>(null);

  const handleFiles = (files: File[]) => {
    for (const file of files) {
      if (file.type.startsWith('image/')) onQueued(file);
    }
  };

  return (
    <div className="scan-panel">
      <div
        className="scan-dropzone"
        onClick={() => inputRef.current?.click()}
        onDragOver={e => e.preventDefault()}
        onDrop={e => { e.preventDefault(); handleFiles(Array.from(e.dataTransfer.files)); }}
      >
        <div className="scan-placeholder">
          <span className="scan-icon">📷</span>
          <span>Drop photos or click to upload</span>
          <span className="scan-hint">Upload multiple — AI scans run in the background</span>
        </div>
      </div>
      <input ref={inputRef} type="file" accept="image/*" capture="environment" multiple style={{ display: 'none' }}
        onChange={e => { handleFiles(Array.from(e.target.files ?? [])); e.target.value = ''; }} />
    </div>
  );
}

// Represents one backend-persisted pending scan (has a real DB id).
interface PendingScanItemProps {
  scan: PendingIngredientScan;
  onAdd: (item: InventoryInput) => Promise<void>;
  onDismiss: (id: number) => void;
}

function PendingScanItem({ scan, onAdd, onDismiss }: PendingScanItemProps) {
  const initial: ItemDraft = {
    name: scan.name || '',
    amount: scan.amount > 0 ? String(scan.amount) : '',
    unit: scan.unit || '',
    notes: scan.confident ? '' : '⚠ Low confidence — please verify',
  };
  const [form, setForm] = useState(initial);
  const set = (f: keyof ItemDraft, v: string) => setForm(prev => ({ ...prev, [f]: v }));
  const [saving, setSaving] = useState(false);

  const handleAdd = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.name.trim()) return;
    setSaving(true);
    try {
      await onAdd({
        name: form.name.trim(),
        amount: parseFloat(form.amount) || 0,
        unit: form.unit.trim(),
        notes: form.notes.trim(),
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <li className="pending-scan-item">
      <div className="pending-scan-header">
        <div className="pending-scan-status">
          <span className="pending-scan-label">{!scan.confident ? 'Review — low confidence' : 'Detected'}</span>
        </div>
        <button className="btn btn-secondary btn-sm" onClick={() => onDismiss(scan.id)}>Dismiss</button>
      </div>
      <form className="inventory-item-form" onSubmit={handleAdd}>
        <div className="inventory-form-row">
          <div className="form-group" style={{ flex: 2 }}>
            <label>Name *</label>
            <input className="edit-input" value={form.name} onChange={e => set('name', e.target.value)} placeholder="e.g. whole milk" />
          </div>
          <div className="form-group" style={{ flex: 1 }}>
            <label>Amount</label>
            <input className="edit-input" type="number" min="0" step="any" value={form.amount} onChange={e => set('amount', e.target.value)} placeholder="0" />
          </div>
          <div className="form-group" style={{ flex: 1 }}>
            <label>Unit</label>
            <input className="edit-input" value={form.unit} onChange={e => set('unit', e.target.value)} placeholder="g, ml, pcs…" />
          </div>
        </div>
        <div className="inventory-form-actions">
          <button className="btn btn-primary" type="submit" disabled={saving || !form.name.trim()}>
            {saving ? 'Adding…' : 'Add to Inventory'}
          </button>
        </div>
      </form>
    </li>
  );
}

interface InFlightScan {
  localId: number;
  error: string | null;
}

export default function InventoryPage() {
  const inventory = useFetch('inventory', listInventory);
  const items = inventory.data ?? [];
  const setItems = inventory.setData;
  const scans = useFetch('inventory-scans', listPendingScans); // backend-persisted
  const pendingScans = scans.data ?? [];
  const reloadScans = scans.reload;
  const [inFlight, setInFlight] = useState<InFlightScan[]>([]); // local placeholders while upload is processing
  const [mode, setMode] = useState<'manual' | 'scan'>('manual');
  const [saving, setSaving] = useState(false);
  const [editingId, setEditingId] = useState<number | null>(null);
  // Bumped after a successful add to remount (and clear) the add form.
  const [addFormKey, setAddFormKey] = useState(0);

  const handleQueued = useCallback((file: File) => {
    const localId = nextLocalId++;
    setInFlight(prev => [...prev, { localId, error: null }]);

    scanIngredient(file)
      .then(() => {
        setInFlight(prev => prev.filter(f => f.localId !== localId));
        reloadScans();
      })
      .catch((err) => {
        setInFlight(prev => prev.map(f => f.localId !== localId ? f : { ...f, error: errorMessage(err) || 'Scan failed' }));
      });
  }, [reloadScans]);

  const handleAdd = async (data: InventoryInput) => {
    setSaving(true);
    try {
      const created = await createInventoryItem(data);
      setItems(prev => [...(prev ?? []), created].sort(byName));
      setAddFormKey(k => k + 1);
      revalidateInventory();
    } catch (err) {
      alert('Failed to save: ' + errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const handleUpdate = async (id: number, data: InventoryInput) => {
    setSaving(true);
    try {
      const updated = await updateInventoryItem(id, data);
      setItems(prev => prev?.map(it => it.id === id ? updated : it));
      setEditingId(null);
      revalidateInventory();
    } catch (err) {
      alert('Failed to update: ' + errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm('Remove this item from inventory?')) return;
    try {
      await deleteInventoryItem(id);
      setItems(prev => prev?.filter(it => it.id !== id));
      revalidateInventory();
    } catch (err) {
      alert('Failed to delete: ' + errorMessage(err));
    }
  };

  const removeScan = (scanId: number) => scans.setData(prev => prev?.filter(s => s.id !== scanId));

  const handlePendingAdd = async (scanId: number, data: InventoryInput) => {
    const created = await createInventoryItem(data);
    setItems(prev => [...(prev ?? []), created].sort(byName));
    await deletePendingScan(scanId);
    removeScan(scanId);
    revalidateInventory();
  };

  const handleDismiss = async (scanId: number) => {
    await deletePendingScan(scanId);
    removeScan(scanId);
  };

  const loadingInventory = inventory.loading && !inventory.data;
  const totalPending = pendingScans.length + inFlight.length;

  return (
    <div className="inventory-page">
      <div className="page-header">
        <h1>Pantry</h1>
      </div>
      <div className="inventory-add-card">
        <div className="inventory-add-header">
          <h3>Add to your pantry</h3>
          <div className="mode-toggle">
            <button type="button" className={mode === 'manual' ? 'active' : ''} onClick={() => setMode('manual')}>Manual</button>
            <button type="button" className={mode === 'scan' ? 'active' : ''} onClick={() => setMode('scan')}>Scan Photo</button>
          </div>
        </div>

        {mode === 'scan' && <ScanPanel onQueued={handleQueued} />}

        {mode === 'manual' && (
          <ItemForm key={addFormKey} initial={EMPTY_FORM} onSave={handleAdd} saving={saving} />
        )}
      </div>

      {totalPending > 0 && (
        <div className="inventory-list-section">
          <h3>Pending Scans <span className="inventory-count">({totalPending})</span></h3>
          <ul className="inventory-list">
            {inFlight.map(f => (
              <li key={`local-${f.localId}`} className="pending-scan-item">
                <div className="pending-scan-header">
                  <div className="pending-scan-status">
                    {f.error
                      ? <span className="pending-scan-error">{f.error}</span>
                      : <><span className="scan-spinner" /><span>Scanning…</span></>}
                  </div>
                  {f.error && (
                    <button className="btn btn-secondary btn-sm"
                      onClick={() => setInFlight(prev => prev.filter(x => x.localId !== f.localId))}>
                      Dismiss
                    </button>
                  )}
                </div>
              </li>
            ))}
            {pendingScans.map(scan => (
              <PendingScanItem
                key={scan.id}
                scan={scan}
                onAdd={(data) => handlePendingAdd(scan.id, data)}
                onDismiss={handleDismiss}
              />
            ))}
          </ul>
        </div>
      )}

      <div className="inventory-list-section">
        <h3>In your pantry {!loadingInventory && <span className="inventory-count">({items.length})</span>}</h3>
        {loadingInventory ? (
          <p className="text-secondary">Loading…</p>
        ) : items.length === 0 ? (
          <p className="text-secondary">No ingredients yet. Add some above.</p>
        ) : (
          <ul className="inventory-list">
            {items.map(item => (
              <li key={item.id} className="inventory-item">
                {editingId === item.id ? (
                  <ItemForm
                    initial={{ name: item.name, amount: item.amount > 0 ? String(item.amount) : '', unit: item.unit, notes: item.notes }}
                    onSave={data => handleUpdate(item.id, data)}
                    onCancel={() => setEditingId(null)}
                    saving={saving}
                  />
                ) : (
                  <div className="inventory-item-row">
                    <div className="inventory-item-info">
                      <span className="inventory-item-name">{item.name}</span>
                      {(item.amount > 0 || item.unit) && (
                        <span className="inventory-item-amount">
                          {item.amount > 0 ? item.amount : ''} {item.unit}
                        </span>
                      )}
                      {item.notes && <span className="inventory-item-notes">{item.notes}</span>}
                    </div>
                    <div className="inventory-item-actions">
                      <button className="btn btn-secondary btn-sm" onClick={() => setEditingId(item.id)}>Edit</button>
                      <button className="btn btn-danger btn-sm" onClick={() => handleDelete(item.id)}>Remove</button>
                    </div>
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
