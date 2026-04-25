import { forwardRef } from 'react'

// Surface-style button with hover-accent border.
// Accepts className to override padding/size/color for variants.
export function Button({ children, className = '', ...props }) {
  return (
    <button
      className={`bg-surface border border-ui-border text-white rounded-[6px] px-[18px] py-[7px] text-[13px] cursor-pointer hover:border-accent transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${className}`}
      {...props}
    >
      {children}
    </button>
  )
}

// Small-caps section heading. Defaults to mb-3.5; pass className="" to suppress.
export function SectionLabel({ children, className = 'mb-3.5' }) {
  return (
    <div className={`text-[11px] text-muted uppercase tracking-[1px] ${className}`}>
      {children}
    </div>
  )
}

// Label + input(s) + optional hint wrapper for form fields.
// Pass labelFor to wire the label's htmlFor. hint accepts ReactNode.
export function TextField({ label, labelFor, hint, children }) {
  return (
    <div className="flex flex-col gap-2">
      <label className="text-[13px] font-medium text-muted" htmlFor={labelFor}>{label}</label>
      {children}
      {hint && <span className="text-[12px] text-muted">{hint}</span>}
    </div>
  )
}

// Scrollable well container for logs, track lists, etc.
// Accepts ref via forwardRef (needed for auto-scroll).
export const Panel = forwardRef(({ children, className = '', ...props }, ref) => (
  <div
    ref={ref}
    className={`bg-well border border-ui-border rounded-[6px] overflow-y-auto p-3 ${className}`}
    {...props}
  >
    {children}
  </div>
))
Panel.displayName = 'Panel'

// A single structured log entry row (structured view, not raw).
export function LogRow({ entry }) {
  return (
    <div className="flex gap-2.5 items-baseline py-0.5">
      <span className="text-[11px] text-muted flex-shrink-0 tabular-nums">{entry.time}</span>
      {entry.level !== 'INFO' && (
        <span className={`text-[10px] font-semibold tracking-wide flex-shrink-0 ${
          entry.level === 'WARN' ? 'text-accent' : entry.level === 'ERROR' ? 'text-danger' : 'text-muted'
        }`}>
          {entry.level}
        </span>
      )}
      <span className="text-[12px] text-white break-words">{entry.msg}</span>
      {entry.track && <span className="text-[12px] text-accent flex-shrink-0">{entry.track}</span>}
      {entry.system && <span className="text-[11px] text-accent flex-shrink-0">{entry.system}</span>}
    </div>
  )
}
