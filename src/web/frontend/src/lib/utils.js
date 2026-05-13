function escHtml(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

export function highlightEnv(text) {
  return text.split('\n').map(line => {
    const trimmed = line.trim()
    if (!trimmed) return ''
    if (trimmed.startsWith('#')) return `<span class="env-comment">${escHtml(line)}</span>`
    const eq = line.indexOf('=')
    if (eq >= 0) {
      const key = line.slice(0, eq)
      const val = line.slice(eq + 1).trim()
      if (!val) return `<span class="env-unset">${escHtml(line)}</span>`
      return `<span class="env-key">${escHtml(key)}</span><span class="env-eq">=</span><span class="env-val">${escHtml(line.slice(eq + 1))}</span>`
    }
    return escHtml(line)
  }).join('\n')
}

export function parseSlogLine(line) {
  const kv = {}
  // Match both plain keys (word chars) and quoted keys ("track title")
  const re = /(\w+|"[^"]+")=("(?:[^"\\]|\\.)*"|[^ ]+)/g
  let m
  while ((m = re.exec(line)) !== null) {
    let [, k, v] = m
    if (k.startsWith('"')) k = k.slice(1, -1) // strip key quotes
    kv[k] = v.startsWith('"') ? v.slice(1, -1).replace(/\\"/g, '"') : v
  }
  if (!kv.msg && !kv.time) return { time: '', level: 'INFO', msg: line }
  let time = ''
  if (kv.time) {
    try { time = new Date(kv.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }) }
    catch { time = kv.time }
  }
  const structural = new Set(['time', 'level', 'msg'])
  const extras = Object.fromEntries(Object.entries(kv).filter(([k]) => !structural.has(k)))

  return {
    time,
    level: (kv.level || 'INFO').toUpperCase(),
    msg: kv.msg || line,
    extras,
  }
}

export function cronToFields(cron) {
  const parts = cron.trim().split(/\s+/)
  // Monthly cron: "m h 1 * *" — day-of-month=1, day-of-week=*
  const isMonthly = parts[2] !== '*' && parts[4] === '*'
  return {
    minute: parseInt(parts[0]) || 0,
    hour: parseInt(parts[1]) || 0,
    day: isMonthly ? 100 : (parts[4] === '*' ? -1 : (parseInt(parts[4]) || 0)),
  }
}
