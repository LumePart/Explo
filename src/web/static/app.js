function escHtml(s) {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function highlightEnv(text) {
  return text.split('\n').map(line => {
    const trimmed = line.trim();
    if (!trimmed) return '';
    if (trimmed.startsWith('#')) return `<span class="env-comment">${escHtml(line)}</span>`;
    const eq = line.indexOf('=');
    if (eq >= 0) {
      const key = line.slice(0, eq);
      const val = line.slice(eq + 1).trim();
      if (!val) return `<span class="env-unset">${escHtml(line)}</span>`;
      return `<span class="env-key">${escHtml(key)}</span><span class="env-eq">=</span><span class="env-val">${escHtml(line.slice(eq + 1))}</span>`;
    }
    return escHtml(line);
  }).join('\n');
}

function parseSlogLine(line) {
  const kv = {};
  const re = /(\w+)=("(?:[^"\\]|\\.)*"|[^ ]+)/g;
  let m;
  while ((m = re.exec(line)) !== null) {
    const [, k, v] = m;
    kv[k] = v.startsWith('"') ? v.slice(1, -1).replace(/\\"/g, '"') : v;
  }
  if (!kv.msg && !kv.time) return { time: '', level: 'INFO', msg: line };
  let time = '';
  if (kv.time) {
    try { time = new Date(kv.time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }); }
    catch { time = kv.time; }
  }
  return { time, level: (kv.level || 'INFO').toUpperCase(), msg: kv.msg || line, track: kv.track || '', system: kv.system || '' };
}

function parseEnv(text) {
  const out = {};
  for (const line of text.split('\n')) {
    const t = line.trim();
    if (!t || t.startsWith('#')) continue;
    const idx = t.indexOf('=');
    if (idx < 0) continue;
    out[t.slice(0, idx).trim()] = t.slice(idx + 1).trim();
  }
  return out;
}

function app() {
  return {
    view: null,
    step: 1,

    // Step 1
    discoveryMode: 'playlist',
    playlists: [
      { value: 'weekly-exploration', name: 'Weekly Exploration', desc: '~50 tracks · refreshes every Tuesday' },
      { value: 'weekly-jams',        name: 'Weekly Jams',        desc: '~25 tracks · refreshes every Monday' },
      { value: 'daily-jams',         name: 'Daily Jams',         desc: '~25 tracks · refreshes daily' },
    ],
    scheduleDays: [
      { value: -1, label: 'Every day', summary: 'Daily' },
      { value: 0, label: 'Sunday', summary: 'Every Sunday' },
      { value: 1, label: 'Monday', summary: 'Every Monday' },
      { value: 2, label: 'Tuesday', summary: 'Every Tuesday' },
      { value: 3, label: 'Wednesday', summary: 'Every Wednesday' },
      { value: 4, label: 'Thursday', summary: 'Every Thursday' },
      { value: 5, label: 'Friday', summary: 'Every Friday' },
      { value: 6, label: 'Saturday', summary: 'Every Saturday' },
    ],
    scheduleKeyMap: {
      'weekly-exploration': 'WEEKLY_EXPLORATION_SCHEDULE',
      'weekly-jams': 'WEEKLY_JAMS_SCHEDULE',
      'daily-jams': 'DAILY_JAMS_SCHEDULE',
    },
    checked: { 'weekly-exploration': true, 'weekly-jams': false, 'daily-jams': false },
    user: '',

    // Step 2
    systems: [
      { value: 'jellyfin',  name: 'Jellyfin' },
      { value: 'emby',      name: 'Emby' },
      { value: 'plex',      name: 'Plex' },
      { value: 'subsonic',  name: 'Subsonic' },
      { value: 'mpd',       name: 'MPD' },
    ],
    system: '',
    systemUrl: '',
    apiKey: '',
    libraryName: '',
    systemUsername: '',
    systemPassword: '',
    playlistDir: '',
    sleepMinutes: '',
    publicPlaylist: false,
    showDirDropdown: false,
    dirSuggestions: [],

    envSources: {},
    saving: false,

    // Step 3
    downloadDir: '',
    useSubdirectory: true,
    showDownloadDirDropdown: false,
    dlServices: { youtube: false, slskd: false },
    youtubeApiKey: '',
    trackExtension: '',
    filterList: '',
    slskdUrl: '',
    slskdApiKey: '',

    // Schedules
    schedules: {
      'weekly-exploration': { enabled: false, day: 2,  hour: 0, minute: 15, editing: false },
      'weekly-jams':        { enabled: false, day: 1,  hour: 0, minute: 30, editing: false },
      'daily-jams':         { enabled: false, day: -1, hour: 1, minute: 15, editing: false },
    },
    scheduleSaveStatus: {},

    // Dashboard
    activeTab: 'run',
    playlist: 'weekly-exploration',
    dlmode: 'normal',
    noPersist: false,
    excludeLocal: false,
    running: false,
    status: '',
    logEntries: [],
    rawLog: false,
    abortController: null,
    rawConfig: '',
    configSaveStatus: '',
    editingConfig: false,
    logFileEntries: [],

    get highlightedConfig() { return highlightEnv(this.rawConfig); },

    get recentTracks() {
      return this.logFileEntries.filter(e => e.track && e.level === 'INFO').reverse();
    },

    get step1Valid() {
      if (this.user.trim() === '') return false;
      if (this.discoveryMode === 'playlist') return Object.values(this.checked).some(Boolean);
      return true;
    },

    get step2Valid() {
      if (!this.system) return false;
      if (this.system === 'mpd') return this.playlistDir.trim() !== '';
      if (!this.systemUrl) return false;
      if (['jellyfin', 'emby', 'plex'].includes(this.system) && !this.apiKey) return false;
      if (this.system === 'subsonic' && (!this.systemUsername || !this.systemPassword)) return false;
      return true;
    },

    get urlPlaceholder() {
      const ports = { jellyfin: '8096', emby: '8096', plex: '32400', subsonic: '4533' };
      return `e.g. http://192.168.1.100:${ports[this.system] || '8096'}`;
    },

    get anyEnvLocked() {
      return this.lockedEnvKeys.length > 0;
    },

    get lockedEnvKeys() {
      return Object.entries(this.envSources)
        .filter(([k, s]) => s === 'env' && !k.endsWith('_SCHEDULE') && !k.endsWith('_FLAGS'))
        .map(([k]) => k);
    },

    isEnvLocked(key) { return this.envSources[key] === 'env'; },

    isScheduleLocked(name) {
      return this.envSources[this.scheduleKeyMap[name]] === 'env';
    },

    scheduleStatusText(name) {
      return this.isScheduleLocked(name) ? 'Set via Docker' : (this.scheduleSaveStatus[name] || '');
    },

    scheduleTime(name) {
      const sched = this.schedules[name];
      return `${String(sched.hour).padStart(2, '0')}:${String(sched.minute).padStart(2, '0')}`;
    },

    updateScheduleTime(name, value) {
      const [hour = '00', minute = '00'] = value.split(':');
      this.schedules[name].hour = parseInt(hour, 10) || 0;
      this.schedules[name].minute = parseInt(minute, 10) || 0;
    },

    scheduleSummary(day) {
      return this.scheduleDays.find(option => option.value === day)?.summary || 'Daily';
    },

    get step3Valid() {
      if (!this.downloadDir.trim()) return false;
      if (!Object.values(this.dlServices).some(Boolean)) return false;
      if (this.dlServices.slskd && (!this.slskdUrl.trim() || !this.slskdApiKey.trim())) return false;
      return true;
    },

    async init() {
      const res = await fetch('/api/config');
      const { values: cfg, sources } = await res.json();
      this.envSources = sources || {};
      if (cfg.LISTENBRAINZ_USER) {
        Object.assign(this, {
          user: cfg.LISTENBRAINZ_USER,
          discoveryMode: cfg.LISTENBRAINZ_DISCOVERY || 'playlist',
          system: cfg.EXPLO_SYSTEM || '',
          systemUrl: cfg.SYSTEM_URL || '',
          apiKey: cfg.API_KEY || '',
          libraryName: cfg.LIBRARY_NAME || '',
          systemUsername: cfg.SYSTEM_USERNAME || '',
          systemPassword: cfg.SYSTEM_PASSWORD || '',
          playlistDir: cfg.PLAYLIST_DIR || '',
          sleepMinutes: cfg.SLEEP || '',
          publicPlaylist: cfg.PUBLIC_PLAYLIST === 'true',
          downloadDir: cfg.DOWNLOAD_DIR || '',
          useSubdirectory: cfg.USE_SUBDIRECTORY !== 'false',
          youtubeApiKey: cfg.YOUTUBE_API_KEY || '',
          trackExtension: cfg.TRACK_EXTENSION || '',
          filterList: cfg.FILTER_LIST || '',
          slskdUrl: cfg.SLSKD_URL || '',
          slskdApiKey: cfg.SLSKD_API_KEY || '',
        });
        this.checked = {
          'weekly-exploration': !!cfg.WEEKLY_EXPLORATION_SCHEDULE,
          'weekly-jams': !!cfg.WEEKLY_JAMS_SCHEDULE,
          'daily-jams': !!cfg.DAILY_JAMS_SCHEDULE,
        };
        const schedMap = {
          'weekly-exploration': cfg.WEEKLY_EXPLORATION_SCHEDULE,
          'weekly-jams':        cfg.WEEKLY_JAMS_SCHEDULE,
          'daily-jams':         cfg.DAILY_JAMS_SCHEDULE,
        };
        for (const [name, cron] of Object.entries(schedMap)) {
          if (cron) {
            const f = this.cronToFields(cron);
            this.schedules[name] = { enabled: true, ...f };
          }
        }
        if (cfg.DOWNLOAD_SERVICES) {
          const s = cfg.DOWNLOAD_SERVICES.split(',');
          this.dlServices = { youtube: s.includes('youtube'), slskd: s.includes('slskd') };
        }
      }
      this.view = cfg.LISTENBRAINZ_USER ? 'settings' : 'wizard';
      await this.refreshRunStatus();
      this.loadLog();
      this.$watch('dlServices.slskd', val => {
        if (val && !this.downloadDir) this.downloadDir = '/slskd/';
      });
    },

    cronToFields(cron) {
      // Parses "MM HH * * DOW" → {minute, hour, day}. DOW "*" = -1 (every day).
      const parts = cron.trim().split(/\s+/);
      return {
        minute: parseInt(parts[0]) || 0,
        hour:   parseInt(parts[1]) || 0,
        day:    parts[4] === '*' ? -1 : (parseInt(parts[4]) || 0),
      };
    },

    nextRunText(sched) {
      if (!sched.enabled) return 'Disabled';
      const hh = String(sched.hour).padStart(2, '0');
      const mm = String(sched.minute).padStart(2, '0');
      return `${this.scheduleSummary(sched.day)} at ${hh}:${mm}`;
    },

    async saveSchedule(name) {
      if (this.isScheduleLocked(name)) {
        this.scheduleSaveStatus[name] = 'Set via Docker';
        return;
      }
      const s = this.schedules[name];
      const res = await fetch('/api/config/schedules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, enabled: s.enabled, day: s.day, hour: s.hour, minute: s.minute }),
      });
      this.scheduleSaveStatus[name] = res.ok ? 'Saved.' : 'Error saving.';
      if (res.ok) {
        setTimeout(() => { this.scheduleSaveStatus[name] = ''; }, 2000);
      }
    },

    async browseDir(input) {
      const path = input.endsWith('/') ? input : (input.includes('/') ? input.slice(0, input.lastIndexOf('/') + 1) : '/');
      const res = await fetch('/api/browse?path=' + encodeURIComponent(path || '/'));
      const all = await res.json();
      this.dirSuggestions = all.filter(d => d.startsWith(input));
    },

    async nextStep() {
      this.saving = true;
      try {
        const playlists = Object.entries(this.checked).filter(([, v]) => v).map(([k]) => k);
        const res = await fetch('/api/wizard/step1', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ user: this.user.trim(), playlists, discovery_mode: this.discoveryMode }),
        });
        if (!res.ok) throw new Error(await res.text());
        this.step = 2;
      } catch (e) {
        alert('Error saving: ' + e.message);
      } finally {
        this.saving = false;
      }
    },

    async submitStep2() {
      this.saving = true;
      try {
        const res = await fetch('/api/wizard/step2', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            system:          this.system,
            url:             this.systemUrl,
            api_key:         this.apiKey,
            library_name:    this.libraryName,
            username:        this.systemUsername,
            password:        this.systemPassword,
            playlist_dir:    this.playlistDir,
            sleep:           this.sleepMinutes,
            public_playlist: this.publicPlaylist,
          }),
        });
        if (!res.ok) throw new Error(await res.text());
        this.step = 3;
      } catch (e) {
        alert('Error saving: ' + e.message);
      } finally {
        this.saving = false;
      }
    },

    async submitStep3() {
      this.saving = true;
      try {
        const services = Object.entries(this.dlServices)
          .filter(([, v]) => v).map(([k]) => k);
        const res = await fetch('/api/wizard/step3', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            download_dir:      this.downloadDir,
            use_subdirectory:  this.useSubdirectory,
            download_services: services,
            youtube_api_key:   this.youtubeApiKey,
            track_extension:   this.trackExtension,
            filter_list:       this.filterList,
            slskd_url:         this.slskdUrl,
            slskd_api_key:     this.slskdApiKey,
          }),
        });
        if (!res.ok) throw new Error(await res.text());
        this.view = 'settings';
      } catch (e) {
        alert('Error saving: ' + e.message);
      } finally {
        this.saving = false;
      }
    },

    async resetConfig() {
      if (!confirm('Reset all settings? This will delete the config file and take you back to setup.')) return;
      await fetch('/api/config/reset', { method: 'POST' });
      location.reload();
    },

    async startRun() {
      this.running = true;
      this.logEntries = [];
      this.status = 'running…';

      const form = new FormData();
      form.set('playlist', this.playlist);
      form.set('download_mode', this.dlmode);
      form.set('persist', this.noPersist ? 'false' : 'true');
      form.set('exclude_local', this.excludeLocal ? 'true' : 'false');

      try {
        const res = await fetch('/api/run', { method: 'POST', body: form });
        if (res.status === 409) {
          this.status = 'already running';
          await this.refreshRunStatus();
          return;
        }
        if (!res.ok) { this.status = 'error'; this.running = false; return; }
        this.followRun();
      } catch (e) {
        this.status = 'error';
        this.running = false;
      }
    },

    async followRun() {
      if (this.abortController) this.abortController.abort();
      const controller = new AbortController();
      this.abortController = controller;

      try {
        const res = await fetch('/api/run/events', { signal: controller.signal });
        if (!res.ok) { this.status = 'error'; this.running = false; return; }

        const reader = res.body.getReader();
        const dec = new TextDecoder();
        let buf = '';
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buf += dec.decode(value, { stream: true });
          const parts = buf.split('\n\n');
          buf = parts.pop();
          for (const part of parts) this.handleRunEvent(part);
        }
      } catch (e) {
        if (e.name !== 'AbortError') {
          this.status = 'error';
          this.running = false;
        }
      } finally {
        if (this.abortController === controller) this.abortController = null;
      }
    },

    handleRunEvent(part) {
      let ev = '', data = '';
      for (const l of part.split('\n')) {
        if (l.startsWith('event: ')) ev = l.slice(7).trim();
        if (l.startsWith('data: ')) data = l.slice(6);
      }
      if (ev === 'done') {
        this.status = parseInt(data) === 0 ? 'done ✓' : 'failed (exit ' + data + ')';
        this.running = false;
      } else if (data) {
        this.logEntries.push({ raw: data, ...parseSlogLine(data) });
        this.$nextTick(() => {
          const el = document.getElementById('log');
          if (el) el.scrollTop = el.scrollHeight;
        });
      }
    },

    async refreshRunStatus() {
      const res = await fetch('/api/run/status');
      if (!res.ok) return;
      const status = await res.json();
      if (status.running) {
        this.running = true;
        this.status = 'running…';
        this.logEntries = [];
        this.followRun();
      }
    },

    async stopRun() {
      if (!this.running) return;
      this.status = 'stopping…';
      const res = await fetch('/api/run/stop', { method: 'POST' });
      if (!res.ok) this.status = 'error stopping run';
    },

    async loadRawConfig() {
      const res = await fetch('/api/config/raw');
      this.rawConfig = await res.text();
      this.editingConfig = false;
    },

    async loadLog() {
      const res = await fetch('/api/logs');
      const text = await res.text();
      this.logFileEntries = text.split('\n').filter(l => l.trim()).map(l => ({ raw: l, ...parseSlogLine(l) }));
    },

    async saveRawConfig() {
      const res = await fetch('/api/config', {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain' },
        body: this.rawConfig,
      });
      if (res.ok) {
        this.editingConfig = false;
        this.configSaveStatus = 'Saved.';
        setTimeout(() => { this.configSaveStatus = ''; }, 2500);
      } else {
        this.configSaveStatus = 'Error saving.';
      }
    },
  };
}
