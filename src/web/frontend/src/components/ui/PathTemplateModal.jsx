import { useState, useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'motion/react'
import { Button } from './common'

export const SEED_PRESETS = [
  { name: 'Artist / Album / Track', template: '{{Artist}}/{{Album}}/{{TrackNumber}} - {{TrackName}}.{{ext}}' },
  { name: 'Year-tagged albums',     template: '{{Artist}}/{{Year}} - {{Album}}/{{TrackNumber}} - {{TrackName}}.{{ext}}' },
  { name: 'Disc-aware',             template: '{{Artist}}/{{Album}}/{{DiscNumber}}-{{TrackNumber}} {{TrackName}}.{{ext}}' },
  { name: 'Flat',                   template: '{{Artist}} - {{TrackName}}.{{ext}}' },
]

const TEMPLATE_VARS = [
  ['Artist', 'Radiohead'],
  ['Album', 'OK Computer'],
  ['TrackName', 'Karma Police'],
  ['TrackNumber', '03'],
  ['DiscNumber', '01'],
  ['Year', '1997'],
  ['File', 'filename'],
  ['ext', 'flac'],
]

const SAMPLE_META = {
  Artist: 'Radiohead', Album: 'OK Computer', AlbumName: 'OK Computer',
  TrackName: 'Karma Police', TrackNumber: '03', DiscNumber: '01',
  Year: '1997', File: 'karma_police', ext: 'flac',
}

function sanitizeSegment(v) {
  return String(v).replace(/[/\\:*?"<>|]/g, '')
}

function resolveTemplate(tpl) {
  return tpl.replace(/\{\{\s*([A-Za-z]+)\s*\}\}/g, (_, name) => {
    const key = Object.keys(SAMPLE_META).find(k => k.toLowerCase() === name.toLowerCase())
    return key ? sanitizeSegment(SAMPLE_META[key]) : `{{${name}}}`
  })
}

export function PathLine({ template }) {
  const parts = resolveTemplate(template).split('/')
  return parts.map((part, i) => {
    const isFile = i === parts.length - 1 && part.includes('.')
    return (
      <span key={i}>
        {i > 0 && <span className="text-white px-[3px]" style={{ opacity: 0.25 }}>/</span>}
        <span className={isFile ? 'text-accent' : 'text-white'}>{part || '·'}</span>
      </span>
    )
  })
}

// Props:
//   onClose  — called on cancel / backdrop / Escape
//   onSave   — called with { name, template } when user saves the preset
export function PathTemplateModal({ onClose, onSave }) {
  const [name, setName] = useState('')
  const [template, setTemplate] = useState(SEED_PRESETS[0].template)
  const nameInputRef = useRef(null)
  const templateInputRef = useRef(null)

  useEffect(() => {
    const handle = e => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', handle)
    setTimeout(() => nameInputRef.current?.focus(), 60)
    return () => window.removeEventListener('keydown', handle)
  }, [onClose])

  const insertVariable = varName => {
    const input = templateInputRef.current
    if (!input) return
    const token = `{{${varName}}}`
    const start = input.selectionStart ?? template.length
    const end = input.selectionEnd ?? template.length
    const next = template.slice(0, start) + token + template.slice(end)
    setTemplate(next)
    const pos = start + token.length
    setTimeout(() => { input.focus(); input.setSelectionRange(pos, pos) }, 0)
  }

  const handleSave = () => {
    onSave({ name: name.trim() || 'Custom template', template })
  }

  return (
    <motion.div
      key="modal-overlay"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.16 }}
      onClick={e => { if (e.target === e.currentTarget) onClose() }}
      style={{
        position: 'fixed', inset: 0, zIndex: 50,
        background: 'rgba(0,0,0,0.72)',
        backdropFilter: 'blur(6px)',
        WebkitBackdropFilter: 'blur(6px)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        padding: 24,
      }}
    >
      <motion.div
        key="modal-dialog"
        initial={{ opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: 8 }}
        transition={{ duration: 0.18 }}
        className="w-full max-w-[540px] border border-ui-border rounded-lg overflow-hidden"
        style={{
          background: '#1a1a1ae6',
          backdropFilter: 'blur(8px)',
          WebkitBackdropFilter: 'blur(8px)',
          boxShadow: '0 24px 64px #00000099',
        }}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-ui-border">
          <span className="text-[15px] font-semibold text-white whitespace-nowrap">New folder template</span>
          <button
            onClick={onClose}
            className="text-muted text-[22px] leading-none cursor-pointer bg-transparent border-none px-1 hover:text-white transition-colors"
          >
            ×
          </button>
        </div>

        {/* Body */}
        <div className="px-5 pt-5 pb-1">
          <p className="text-[11px] text-muted uppercase tracking-[1px] mb-2">Template name</p>
          <input
            ref={nameInputRef}
            className="w-full bg-well border border-ui-border text-white rounded-[6px] px-3 py-2.5 text-[14px] outline-none focus:border-accent transition-colors"
            placeholder="e.g. By genre"
            value={name}
            onChange={e => setName(e.target.value)}
            spellCheck={false}
          />

          <p className="text-[11px] text-muted uppercase tracking-[1px] mb-2 mt-5">Folder structure</p>
          <input
            ref={templateInputRef}
            className="w-full bg-well border border-ui-border text-white rounded-[6px] px-3 py-2.5 text-[13px] outline-none focus:border-accent transition-colors"
            value={template}
            onChange={e => setTemplate(e.target.value)}
            spellCheck={false}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
          />

          <p className="text-[11px] text-muted uppercase tracking-[1px] mb-2 mt-5">Preview</p>
          <div className="flex items-baseline gap-2.5 overflow-x-auto py-1">
            <span className="text-white shrink-0" style={{ opacity: 0.25 }}>→</span>
            <div className="text-[13px] font-medium whitespace-nowrap">
              <PathLine template={template} />
            </div>
          </div>

          <p className="text-[11px] text-muted uppercase tracking-[1px] mb-2 mt-5">Insert variable</p>
          <div className="flex flex-wrap gap-2">
            {TEMPLATE_VARS.map(([varName, example]) => (
              <button
                key={varName}
                onClick={() => insertVariable(varName)}
                className="flex items-baseline gap-1.5 bg-surface border border-ui-border rounded-[6px] px-2.5 py-1.5 text-[12px] text-white cursor-pointer transition-colors hover:border-accent hover:text-accent"
              >
                {varName}
                <span className="text-[10px] text-muted">{example}</span>
              </button>
            ))}
          </div>

          <p className="text-[11px] text-muted mt-3 mb-5 leading-relaxed">
            Click to insert at cursor. Illegal characters{' '}
            {['/', '\\', ':', '*', '?', '"', '<', '>', '|'].map(c => (
              <span key={c} className="inline-block bg-surface border border-ui-border rounded px-1 text-[10px] mx-0.5">{c}</span>
            ))}{' '}
            in a value are stripped automatically.
          </p>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 px-5 pb-5">
          <button
            onClick={onClose}
            className="bg-transparent border-none text-muted text-[13px] cursor-pointer p-0 hover:text-white transition-colors"
          >
            Cancel
          </button>
          <Button onClick={handleSave} style={{ background: 'transparent' }}>
            Save preset
          </Button>
        </div>
      </motion.div>
    </motion.div>
  )
}
