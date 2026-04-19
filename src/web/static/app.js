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
    playlists: [
      { value: 'weekly-exploration', name: 'Weekly Exploration', desc: '~50 tracks · refreshes every Tuesday' },
      { value: 'weekly-jams',        name: 'Weekly Jams',        desc: '~25 tracks · refreshes every Monday' },
      { value: 'daily-jams',         name: 'Daily Jams',         desc: '~25 tracks · refreshes daily' },
    ],
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

    saving: false,

    // Step 3
    dlServices: { youtube: true, slskd: false },
    youtubeApiKey: '',
    trackExtension: '',
    filterList: '',
    slskdUrl: '',
    slskdApiKey: '',

    // Dashboard
    activeTab: 'run',
    playlist: 'weekly-exploration',
    dlmode: 'normal',
    noPersist: false,
    excludeLocal: false,
    running: false,
    status: '',
    log: '',

    get step1Valid() {
      return this.user.trim() !== '' && Object.values(this.checked).some(Boolean);
    },

    get step2Valid() {
      if (!this.system) return false;
      if (this.system === 'mpd') return this.playlistDir.trim() !== '';
      if (!this.systemUrl) return false;
      if (['jellyfin', 'emby', 'plex'].includes(this.system) && !this.apiKey) return false;
      if (this.system === 'subsonic' && (!this.systemUsername || !this.systemPassword)) return false;
      return true;
    },

    get step3Valid() {
      if (!Object.values(this.dlServices).some(Boolean)) return false;
      if (this.dlServices.youtube && !this.youtubeApiKey.trim()) return false;
      if (this.dlServices.slskd && (!this.slskdUrl.trim() || !this.slskdApiKey.trim())) return false;
      return true;
    },

    async init() {
      const res = await fetch('/api/config');
      const cfg = parseEnv(await res.text());
      this.view = cfg['LISTENBRAINZ_USER'] ? 'settings' : 'wizard';
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
          body: JSON.stringify({ user: this.user.trim(), playlists }),
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

    async startRun() {
      this.running = true;
      this.log = '';
      this.status = 'running…';

      const form = new FormData();
      form.set('playlist', this.playlist);
      form.set('download_mode', this.dlmode);
      form.set('persist', this.noPersist ? 'false' : 'true');
      form.set('exclude_local', this.excludeLocal ? 'true' : 'false');

      const res = await fetch('/api/run', { method: 'POST', body: form });
      if (res.status === 409) { this.log = 'already running'; this.running = false; return; }
      if (!res.ok) { this.log = 'error: ' + await res.text(); this.running = false; return; }

      const reader = res.body.getReader();
      const dec = new TextDecoder();
      let buf = '';
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        const parts = buf.split('\n\n');
        buf = parts.pop();
        for (const part of parts) {
          let ev = '', data = '';
          for (const l of part.split('\n')) {
            if (l.startsWith('event: ')) ev = l.slice(7).trim();
            if (l.startsWith('data: ')) data = l.slice(6);
          }
          if (ev === 'done') {
            this.status = parseInt(data) === 0 ? 'done ✓' : 'failed (exit ' + data + ')';
            this.running = false;
          } else if (data) {
            this.log += data + '\n';
            this.$nextTick(() => {
              const el = document.getElementById('log');
              if (el) el.scrollTop = el.scrollHeight;
            });
          }
        }
      }
    },
  };
}
