import { useState } from 'react'
import { fetchBrowse } from '../../lib/api'

export function DirInput({ value, onChange, disabled, placeholder }) {
  const [show, setShow] = useState(false)
  const [suggestions, setSuggestions] = useState([])

  async function browse(input) {
    const path = input.endsWith('/') ? input : (input.includes('/') ? input.slice(0, input.lastIndexOf('/') + 1) : '/')
    try {
      const all = await fetchBrowse(path || '/')
      setSuggestions(all.filter(d => d.startsWith(input)))
    } catch {
      setSuggestions([])
    }
  }

  return (
    <div className="relative">
      <input
        type="text"
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        autoComplete="off"
        onChange={e => { onChange(e.target.value); browse(e.target.value) }}
        onFocus={() => { browse(value); setShow(true) }}
        onBlur={() => setTimeout(() => setShow(false), 150)}
        className="w-full bg-surface border border-ui-border text-white rounded-[6px] px-3 py-2.5 text-[15px] outline-none focus:border-accent disabled:opacity-45 disabled:cursor-not-allowed transition-colors"
      />
      {show && suggestions.length > 0 && (
        <ul className="absolute top-full left-0 right-0 z-10 bg-surface border border-accent border-t-0 rounded-b-[6px] max-h-[180px] overflow-y-auto list-none">
          {suggestions.map(d => (
            <li
              key={d}
              onMouseDown={e => { e.preventDefault(); onChange(d + '/'); browse(d + '/') }}
              className="px-3 py-2 text-[13px] text-muted cursor-pointer font-mono hover:bg-[#242424] hover:text-white transition-colors"
            >
              {d}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
