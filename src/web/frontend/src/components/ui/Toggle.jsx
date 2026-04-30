export function Toggle({ checked, onChange, disabled, small, tiny }) {
  if (tiny) return (
    <div
      className={`w-[26px] h-[15px] rounded-full relative flex-shrink-0 transition-colors
        ${checked ? 'bg-accent' : 'bg-ui-border'}
        ${disabled ? 'opacity-45' : 'cursor-pointer'}`}
      onClick={() => !disabled && onChange(!checked)}
    >
      <span
        className={`absolute top-[2px] left-[2px] w-[11px] h-[11px] bg-white rounded-full transition-transform
          ${checked ? 'translate-x-[11px]' : 'translate-x-0'}`}
      />
    </div>
  )
  if (small) return (
    <div
      className={`w-6 h-3.5 rounded-full relative flex-shrink-0 transition-colors
        ${checked ? 'bg-accent' : 'bg-ui-border'}
        ${disabled ? 'opacity-45' : 'cursor-pointer'}`}
      onClick={() => !disabled && onChange(!checked)}
    >
      <span
        className={`absolute top-[2px] left-[2px] w-2.5 h-2.5 bg-white rounded-full transition-transform
          ${checked ? 'translate-x-[10px]' : 'translate-x-0'}`}
      />
    </div>
  )
  return (
    <div
      className={`w-9 h-5 rounded-full relative flex-shrink-0 transition-colors
        ${checked ? 'bg-accent' : 'bg-ui-border'}
        ${disabled ? 'opacity-45' : 'cursor-pointer'}`}
      onClick={() => !disabled && onChange(!checked)}
    >
      <span
        className={`absolute top-[3px] left-[3px] w-3.5 h-3.5 bg-white rounded-full transition-transform
          ${checked ? 'translate-x-4' : 'translate-x-0'}`}
      />
    </div>
  )
}

export function ToggleRow({ checked, onChange, disabled, name, desc, children }) {
  return (
    <label
      className={`flex items-center gap-3 px-4 py-3.5 bg-surface border rounded-[6px] select-none
        ${checked ? 'border-accent' : 'border-ui-border hover:border-[#404040]'}
        ${disabled ? 'opacity-45 cursor-not-allowed pointer-events-none' : 'cursor-pointer'}
        transition-colors`}
    >
      <div className="flex-1 flex flex-col gap-[3px]">
        {name && <span className="text-[14px] font-medium">{name}</span>}
        {desc && <span className="text-[12px] text-muted">{desc}</span>}
        {children}
      </div>
      <Toggle checked={checked} onChange={onChange} disabled={disabled} />
    </label>
  )
}
