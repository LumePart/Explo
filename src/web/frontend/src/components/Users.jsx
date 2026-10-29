import { useState, useEffect, useCallback } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { fetchUsers, createUser, updateUser, deleteUser, fetchConfig } from '../lib/api'
import { Toggle } from './ui/Toggle'
import { Button, TextField } from './ui/common'

const inputCls = 'w-full bg-well border border-ui-border text-white rounded-lg px-3 py-2 text-[13px] outline-none placeholder:text-[#444] focus:border-accent transition-colors disabled:opacity-50'

function Modal({ onClose, busy, children }) {
  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.2 }}
      onClick={busy ? undefined : onClose}
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ background: 'rgba(0, 0, 0, 0.72)', backdropFilter: 'blur(6px)', WebkitBackdropFilter: 'blur(6px)' }}
    >
      <motion.div
        initial={{ opacity: 0, y: 24, scale: 0.97 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        exit={{ opacity: 0, y: 16, scale: 0.97 }}
        transition={{ duration: 0.28, ease: [0.16, 1, 0.3, 1] }}
        onClick={e => e.stopPropagation()}
        className="w-full max-w-[420px] mx-4 border border-ui-border rounded-lg overflow-hidden"
        style={{ background: '#0d0d0df0', backdropFilter: 'blur(8px)', WebkitBackdropFilter: 'blur(8px)', boxShadow: '0 24px 64px #00000099' }}
      >
        {children}
      </motion.div>
    </motion.div>
  )
}

function UserForm({ user, prefill, onClose, onSaved }) {
  const editing = !!user
  const [name, setName] = useState(user?.name ?? '')
  const [lbUser, setLbUser] = useState(user?.listenbrainz_user ?? prefill?.lbUser ?? '')
  const [token, setToken] = useState(prefill?.token ?? '')
  const [plexUser, setPlexUser] = useState(user?.plex_user ?? prefill?.plexUser ?? '')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  const save = async () => {
    const body = {
      name: name.trim(),
      listenbrainz_user: lbUser.trim(),
      listenbrainz_token: token.trim(),
      plex_user: plexUser.trim(),
      enabled: user?.enabled ?? true,
    }
    if (!body.name) return setError('Name is required.')
    if (!body.listenbrainz_user) return setError('ListenBrainz username is required.')
    if (!body.plex_user) return setError('Plex username is required.')
    setSaving(true)
    setError('')
    try {
      if (editing) await updateUser(user.id, body)
      else await createUser(body)
      onSaved()
    } catch (e) {
      setError(e.message || 'Save failed.')
      setSaving(false)
    }
  }

  return (
    <Modal onClose={onClose} busy={saving}>
      <div className="flex items-center justify-between px-5 py-4 border-b border-ui-border">
        <h2 className="text-[15px] font-semibold text-white leading-none">{editing ? 'Edit user' : 'Add user'}</h2>
        <button
          onClick={onClose}
          disabled={saving}
          className="text-muted hover:text-white transition-colors text-[18px] leading-none p-0 bg-transparent border-none cursor-pointer disabled:opacity-40"
        >
          &times;
        </button>
      </div>
      <div className="px-5 pt-5 pb-5 flex flex-col gap-4">
        {prefill && <p className="text-[12px] text-muted -mt-1">Pre-filled from your current settings.</p>}
        <TextField
          label="Display name"
          labelFor="user-name"
          hint={editing ? undefined : "The id is generated from the name and can't change later."}
        >
          <input id="user-name" className={inputCls} value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Sam" disabled={saving} autoFocus />
        </TextField>
        <TextField label="ListenBrainz username" labelFor="user-lb">
          <input id="user-lb" className={inputCls} value={lbUser} onChange={e => setLbUser(e.target.value)} disabled={saving} />
        </TextField>
        <TextField
          label="ListenBrainz token"
          labelFor="user-token"
          hint={editing && user.token_set ? 'A token is set. Leave blank to keep it.' : undefined}
        >
          <input
            id="user-token"
            type="password"
            autoComplete="new-password"
            className={inputCls}
            value={token}
            onChange={e => setToken(e.target.value)}
            placeholder={editing && user.token_set ? '•••••••• (set)' : 'paste token'}
            disabled={saving}
          />
        </TextField>
        <TextField label="Plex username" labelFor="user-plex">
          <input id="user-plex" className={inputCls} value={plexUser} onChange={e => setPlexUser(e.target.value)} disabled={saving} />
        </TextField>
        {error && <p className="text-[12px] text-danger">{error}</p>}
      </div>
      <div className="flex justify-end gap-2 px-5 pb-5">
        <Button onClick={onClose} disabled={saving} className="!rounded-full !px-4 !py-1.5 text-muted">Cancel</Button>
        <Button onClick={save} disabled={saving} className="!rounded-full !px-5 !py-1.5 !border-accent !text-accent">
          {saving ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </Modal>
  )
}

function DeleteConfirm({ user, onClose, onDeleted }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const confirm = async () => {
    setBusy(true)
    setError('')
    try {
      await deleteUser(user.id)
      onDeleted()
    } catch (e) {
      setError(e.message || 'Delete failed.')
      setBusy(false)
    }
  }

  return (
    <Modal onClose={onClose} busy={busy}>
      <div className="px-5 py-4 border-b border-ui-border">
        <h2 className="text-[15px] font-semibold text-white leading-none">Delete {user.name}?</h2>
        <p className="text-[12px] text-muted mt-2">
          This removes {user.name}&apos;s record and cached playlists from Explo. Nothing in Plex is touched.
        </p>
        {error && <p className="text-[12px] text-danger mt-2">{error}</p>}
      </div>
      <div className="flex justify-end gap-2 px-5 py-4">
        <Button onClick={onClose} disabled={busy} className="!rounded-full !px-4 !py-1.5 text-muted">Cancel</Button>
        <Button onClick={confirm} disabled={busy} className="!rounded-full !px-5 !py-1.5 !border-danger !text-danger">
          {busy ? 'Deleting…' : 'Delete'}
        </Button>
      </div>
    </Modal>
  )
}

// ── Users Tab ─────────────────────────────────────────────────────────────────
// Manage users for multi-user mode. Tokens are write-only: the API only
// reports whether one is set.

export function Users() {
  const [users, setUsers] = useState(null)
  const [loadError, setLoadError] = useState('')
  const [actionError, setActionError] = useState('')
  const [adminMissing, setAdminMissing] = useState(false)
  const [form, setForm] = useState(null)       // null | 'new' | user
  const [deleting, setDeleting] = useState(null)
  const [legacy, setLegacy] = useState(null)   // single-user settings, used to pre-fill the first user

  const load = useCallback(() => {
    return fetchUsers()
      .then(list => { setUsers(list ?? []); setLoadError('') })
      .catch(e => setLoadError(e.message || 'Failed to load users.'))
  }, [])

  useEffect(() => {
    load()
    // Only an explicit false shows the banner; absent or failed means unknown.
    fetchConfig().then(cfg => {
      setAdminMissing(cfg.admin_configured === false)
      const v = cfg.values ?? {}
      const found = {
        lbUser: v.LISTENBRAINZ_USER || '',
        token: v.LISTENBRAINZ_USER_TOKEN || '',
        plexUser: v.SYSTEM_USERNAME || '',
      }
      setLegacy(found.lbUser || found.token || found.plexUser ? found : null)
    }).catch(() => {})
  }, [load])

  const toggleEnabled = async u => {
    setActionError('')
    try {
      await updateUser(u.id, {
        name: u.name,
        listenbrainz_user: u.listenbrainz_user,
        listenbrainz_token: '',
        plex_user: u.plex_user,
        enabled: !u.enabled,
      })
      await load()
    } catch (e) {
      setActionError(e.message || 'Update failed.')
    }
  }

  const list = users ?? []
  const enabledUsers = list.filter(u => u.enabled)

  return (
    <div>
      {adminMissing && list.length > 0 && (
        <div className="mb-4 px-4 py-3 bg-surface border border-danger rounded-[6px] text-[12px] text-muted">
          <strong className="text-danger font-semibold">Plex admin credentials are missing.</strong>{' '}
          Users can&apos;t be switched to without them. Add an admin API key (or admin username and password) in Settings.
        </div>
      )}

      <div className="mt-6">
        <div className="flex items-center justify-between">
          <div className="text-[16px] font-bold tracking-tight text-white">Users</div>
          <button
            onClick={() => setForm('new')}
            className="bg-transparent border border-ui-border text-muted rounded-full px-3 py-1 text-[12px] cursor-pointer hover:text-white hover:border-[#444] transition-colors"
          >
            + Add user
          </button>
        </div>

        {loadError && <p className="text-[12px] text-danger mt-3">{loadError}</p>}
        {actionError && <p className="text-[12px] text-danger mt-3">{actionError}</p>}

        {users && list.length === 0 && !loadError ? (
          <p className="text-[12px] text-muted mt-3">
            No users yet. Add one to switch this instance to multi-user. Until then everything works as it does today.
          </p>
        ) : (
          <div className="flex flex-col gap-3 mt-3">
            {list.map(u => {
              const others = enabledUsers.filter(o => o.id !== u.id)
              const sharing = u.enabled && enabledUsers.length > 1
              return (
                <div key={u.id} className="bg-surface border border-ui-border rounded-[6px] px-4 py-3.5 flex items-center gap-3.5 flex-wrap">
                  <div className="flex-1 min-w-[200px]">
                    <div className="flex items-center gap-2">
                      <span className="text-[14px] font-semibold text-white">{u.name}</span>
                      <span className="text-[11px] text-muted bg-well border border-ui-border rounded px-1.5 py-px">{u.id}</span>
                    </div>
                    <div className="text-[12px] text-muted mt-0.5">
                      ListenBrainz {u.listenbrainz_user} &middot; token{' '}
                      {u.token_set ? 'set' : <span className="text-danger">not set</span>} &middot; Plex {u.plex_user}
                    </div>
                    <div className="text-[12px] text-[#565656] mt-0.5">
                      {sharing
                        ? `Copies shared with ${others.map(o => o.name).join(', ')}`
                        : 'Sharing needs two or more enabled users'}
                    </div>
                  </div>
                  <span className="text-[12px] text-muted">Enabled</span>
                  <Toggle checked={u.enabled} onChange={() => toggleEnabled(u)} />
                  <Button className="!px-3 !py-1 text-[12px]" onClick={() => setForm(u)}>Edit</Button>
                  <Button className="!px-3 !py-1 text-[12px] hover:!border-danger text-danger" onClick={() => setDeleting(u)}>Delete</Button>
                </div>
              )
            })}
          </div>
        )}

        <p className="text-[12px] text-muted mt-3">
          Deleting a user removes their record and cached playlists from Explo. Nothing in Plex is touched.
        </p>
      </div>

      <AnimatePresence>
        {form && (
          <UserForm
            key="form"
            user={form === 'new' ? null : form}
            prefill={form === 'new' && list.length === 0 ? legacy : null}
            onClose={() => setForm(null)}
            onSaved={() => { setForm(null); load() }}
          />
        )}
        {deleting && (
          <DeleteConfirm
            key="delete"
            user={deleting}
            onClose={() => setDeleting(null)}
            onDeleted={() => { setDeleting(null); load() }}
          />
        )}
      </AnimatePresence>
    </div>
  )
}
