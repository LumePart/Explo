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
      return Object.values(this.envSources).some(s => s === 'env');
    },

    isEnvLocked(key) { return this.envSources[key] === 'env'; },

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
        if (cfg.DOWNLOAD_SERVICES) {
          const s = cfg.DOWNLOAD_SERVICES.split(',');
          this.dlServices = { youtube: s.includes('youtube'), slskd: s.includes('slskd') };
        }
      }
      this.view = cfg.LISTENBRAINZ_USER ? 'settings' : 'wizard';
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
