(() => {
  const $ = (selector, root = document) => root?.querySelector(selector) || null;
  const $$ = (selector, root = document) => root ? [...root.querySelectorAll(selector)] : [];

  const formatBytes = (value) => {
    let n = Number(value || 0);
    if (n < 1024) return `${n} B`;
    const units = ['KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
    let i = -1;
    do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
    return `${n.toFixed(1)} ${units[i]}`;
  };
  const formatTime = (value) => {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString();
  };

  // Progreso de indexación en vivo.
  const indexCards = $$('[data-index-card]');
  const settingsIndexCards = $$('[data-settings-index-card]');
  if (indexCards.length || settingsIndexCards.length) {
    const updateIndexProgress = async () => {
      try {
        const response = await fetch('/api/indexacion', { headers: { Accept: 'application/json' }, cache: 'no-store' });
        if (!response.ok) return;
        const jobs = await response.json();
        const byID = new Map(jobs.map((job) => [job.storage_id, job]));
        indexCards.forEach((card) => {
          const job = byID.get(card.dataset.indexCard);
          if (!job) return;
          const box = $('[data-index-progress]', card);
          const bar = $('[data-index-bar]', card);
          const label = $('[data-index-label]', card);
          const percent = $('[data-index-percent]', card);
          const detail = $('[data-index-detail]', card);
          const button = $('[data-index-button]', card);
          const active = ['queued', 'counting', 'scanning'].includes(job.state);
          box?.classList.toggle('active', active);
          if (button) button.disabled = active;
          if (bar) bar.value = job.percent || 0;
          if (percent) percent.textContent = `${job.percent || 0}%`;
          if (label) {
            if (job.state === 'queued') label.textContent = 'En cola…';
            else if (job.state === 'counting') label.textContent = 'Contando archivos…';
            else if (job.state === 'scanning') label.textContent = `Indexando ${job.scanned} de ${job.total}`;
            else if (job.state === 'done') label.textContent = `Indexación completa: ${job.scanned} archivos`;
            else if (job.state === 'error') label.textContent = `Error: ${job.error}`;
          }
          if (detail) {
            if (job.state === 'scanning') detail.textContent = `${job.images || 0} imágenes · ${job.videos || 0} videos · ${job.audio || 0} audios${job.damaged ? ` · ${job.damaged} dañados` : ''}`;
            else if (job.state === 'done') detail.textContent = `Última indexación terminada${job.damaged ? ` · ${job.damaged} dañados detectados` : ''}`;
            else detail.textContent = '';
          }
          const damagedPanel = $('[data-damaged-panel]', card);
          const damagedCount = $('[data-damaged-count]', card);
          if (job.state === 'done' && damagedPanel) {
            damagedPanel.hidden = !(job.damaged > 0);
            if (damagedCount) damagedCount.textContent = String(job.damaged || 0);
          }
        });
        settingsIndexCards.forEach((card) => {
          const job = byID.get(card.dataset.settingsIndexCard);
          if (!job) return;
          const bar = $('[data-settings-index-bar]', card);
          const label = $('[data-settings-index-label]', card);
          const percent = $('[data-settings-index-percent]', card);
          const detail = $('[data-settings-index-detail]', card);
          if (bar) bar.value = job.percent || 0;
          if (percent) percent.textContent = `${job.percent || 0}%`;
          if (label) {
            if (job.state === 'queued') label.textContent = 'En cola…';
            else if (job.state === 'counting') label.textContent = job.verify_all ? 'Contando para verificar integridad…' : 'Contando archivos…';
            else if (job.state === 'scanning') label.textContent = `${job.verify_all ? 'Verificando' : 'Sincronizando'} ${job.scanned || 0} / ${job.total || 0}`;
            else if (job.state === 'done') label.textContent = job.verify_all ? 'Verificación de integridad completa' : 'Sincronización completa';
            else if (job.state === 'error') label.textContent = `Error: ${job.error || 'desconocido'}`;
          }
          if (detail) detail.textContent = `+${job.added || 0} nuevos · ${job.changed || 0} modificados · ${job.removed || 0} eliminados · ${job.damaged || 0} dañados · ${job.unchecked || 0} sin verificar`;
          const previousState = card.dataset.jobState || '';
          card.dataset.jobState = job.state || '';
          if (['queued','counting','scanning'].includes(previousState) && job.state === 'done') window.setTimeout(() => window.location.reload(), 350);
        });
      } catch (_) {}
    };
    updateIndexProgress();
    window.setInterval(updateIndexProgress, 1000);
  }

  // Widgets de diálogo.
  const uploadDialog = $('[data-upload-dialog]');
  $('[data-open-upload]')?.addEventListener('click', () => uploadDialog?.showModal());
  $('[data-close-upload]')?.addEventListener('click', () => uploadDialog?.close());
  uploadDialog?.addEventListener('click', (event) => { if (event.target === uploadDialog) uploadDialog.close(); });

  const filterDialog = $('[data-gallery-filter-dialog]');
  $('[data-open-gallery-filter]')?.addEventListener('click', () => filterDialog?.showModal());
  $('[data-close-gallery-filter]')?.addEventListener('click', () => filterDialog?.close());
  filterDialog?.addEventListener('click', (event) => { if (event.target === filterDialog) filterDialog.close(); });
  $$('[data-icon-select]').forEach((select) => {
    const wrapper = select.closest('.select-with-icon');
    const refresh = () => {
      const value = select.value;
      $$('[data-select-icon]', wrapper).forEach((icon) => { icon.hidden = icon.dataset.selectIcon !== value; });
    };
    select.addEventListener('change', refresh);
    refresh();
  });

  // Descargas seguras mediante ticket opaco de vida corta.
  const csrfToken = $('meta[name="csrf-token"]')?.content || '';
  const downloadMenu = $('[data-download-menu]');
  let downloadTargetID = '';
  const hideDownloadMenu = () => {
    if (!downloadMenu) return;
    downloadMenu.hidden = true;
    downloadTargetID = '';
  };
  const showDownloadMenu = (x, y, fileID) => {
    if (!downloadMenu || !fileID) return;
    downloadTargetID = fileID;
    downloadMenu.hidden = false;
    downloadMenu.style.left = '0px';
    downloadMenu.style.top = '0px';
    const rect = downloadMenu.getBoundingClientRect();
    const left = Math.max(8, Math.min(x, window.innerWidth - rect.width - 8));
    const top = Math.max(8, Math.min(y, window.innerHeight - rect.height - 8));
    downloadMenu.style.left = `${left}px`;
    downloadMenu.style.top = `${top}px`;
  };
  const startSecureDownload = async (fileID) => {
    if (!fileID || !csrfToken) return;
    const response = await fetch('/api/descargas', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8', Accept: 'application/json' },
      body: new URLSearchParams({ csrf_token: csrfToken, file_id: fileID }),
      cache: 'no-store'
    });
    if (!response.ok || !response.headers.get('content-type')?.includes('application/json')) {
      const message = (await response.text()).trim() || 'No se pudo preparar la descarga.';
      throw new Error(message);
    }
    const data = await response.json();
    if (!data.url) throw new Error('El servidor no devolvió una descarga válida.');
    const anchor = document.createElement('a');
    anchor.href = data.url;
    anchor.hidden = true;
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
  };
  $('[data-secure-download]', downloadMenu)?.addEventListener('click', async () => {
    const id = downloadTargetID;
    hideDownloadMenu();
    try { await startSecureDownload(id); } catch (error) { window.alert(error.message || 'No se pudo descargar el archivo.'); }
  });
  document.addEventListener('click', (event) => { if (!event.target.closest('[data-download-menu]')) hideDownloadMenu(); });
  window.addEventListener('blur', hideDownloadMenu);
  window.addEventListener('resize', hideDownloadMenu);

  // Galería y visor multimedia offline.
  const galleryPage = $('[data-gallery-page]');
  const grid = $('[data-gallery-grid]');
  const sentinel = $('[data-gallery-sentinel]');
  const viewer = $('[data-media-viewer]');
  const viewerShell = $('[data-viewer-shell]', viewer);
  const stage = $('[data-viewer-stage]', viewer);
  const videoControls = $('[data-video-controls]', viewer);
  const videoPlayButton = $('[data-video-play]', viewer);
  const videoPlayIcon = $('[data-video-play-icon]', viewer);
  const videoPauseIcon = $('[data-video-pause-icon]', viewer);
  const videoProgress = $('[data-video-progress]', viewer);
  const videoCurrent = $('[data-video-current]', viewer);
  const videoDuration = $('[data-video-duration]', viewer);
  const videoMuteButton = $('[data-video-mute]', viewer);
  const videoVolume = $('[data-video-volume]', viewer);
  const videoSpeed = $('[data-video-speed]', viewer);
  const fullscreenButton = $('[data-viewer-fullscreen]', viewer);
  const qualityControl = $('[data-video-quality-control]', viewer);
  const qualitySelect = $('[data-video-quality]', viewer);
  const qualityStatus = $('[data-video-quality-status]', viewer);
  let currentIndex = -1;
  let currentMediaID = '';
  let activeVideo = null;
  let zoom = 1;
  let loadingGallery = false;
  let availabilityBaseline = null;

  const mediaCards = () => $$('[data-media-id]', grid || document);
  const makeMediaCard = (item) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = `photo-card media-card${item.health === 'damaged' ? ' is-damaged' : ''}`;
    Object.assign(button.dataset, {
      mediaId: item.id,
      selectableFile: item.id,
      downloadFileId: item.id,
      storageId: item.storage_id,
      kind: item.kind,
      name: item.name,
      original: item.original_url,
      preview: item.preview_url || '',
      thumbnail: item.thumbnail_url || '',
      cacheVersion: String(item.cache_version || 0)
    });
    button.title = `Abrir ${item.name}`;
    if (item.thumbnail_url) {
      const image = document.createElement('img');
      image.src = item.thumbnail_url;
      image.loading = 'lazy';
      image.alt = '';
      button.append(image);
    } else {
      const placeholder = document.createElement('div');
      placeholder.className = 'photo-placeholder';
      placeholder.textContent = item.kind === 'video' ? '▶' : item.kind === 'audio' ? '♪' : '▧';
      button.append(placeholder);
    }
    const check = document.createElement('span');
    check.className = 'selection-check';
    check.setAttribute('aria-hidden', 'true');
    check.textContent = '✓';
    button.append(check);
    if (item.health === 'damaged' || item.kind === 'video' || item.kind === 'audio') {
      const badge = document.createElement('span');
      badge.className = `media-badge${item.health === 'damaged' ? ' media-badge-danger' : ''}`;
      badge.textContent = item.health === 'damaged' ? 'Dañado' : item.kind === 'video' ? 'Video' : 'Audio';
      button.append(badge);
    }
    const meta = document.createElement('div');
    meta.className = 'photo-meta';
    const strong = document.createElement('strong');
    strong.textContent = item.name;
    const small = document.createElement('small');
    small.textContent = `${formatTime(item.mod_time)} · ${formatBytes(item.size)}`;
    meta.append(strong, small);
    button.append(meta);
    return button;
  };
  const galleryAPIURL = (offset) => {
    const query = new URLSearchParams(window.location.search);
    query.delete('pagina');
    query.delete('modo');
    query.set('offset', String(offset));
    query.set('limit', '80');
    return `/api/galeria?${query.toString()}`;
  };
  const loadMoreGallery = async () => {
    if (!grid || loadingGallery || grid.dataset.hasMore !== 'true') return;
    loadingGallery = true;
    try {
      const offset = Number(grid.dataset.next || 0);
      const response = await fetch(galleryAPIURL(offset), { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) return;
      const data = await response.json();
      data.items.forEach((item) => grid.append(makeMediaCard(item)));
      grid.dataset.next = data.next;
      grid.dataset.hasMore = String(data.has_more);
      if (sentinel && !data.has_more) sentinel.textContent = 'Fin de la galería';
    } finally {
      loadingGallery = false;
    }
  };
  if (grid?.dataset.mode === 'infinito' && sentinel) {
    new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) loadMoreGallery();
    }, { rootMargin: '500px' }).observe(sentinel);
  }

  const VIDEO_PREF_KEY = 'pc_video_preferences_v1';
  const viewerLoader = $('[data-viewer-loader]', viewer);
  const viewerLoadingText = $('[data-viewer-loading-text]', viewer);
  const connectionInfo = navigator.connection || navigator.mozConnection || navigator.webkitConnection || null;
  let videoProgressFrame = 0;
  let videoScrubbing = false;
  let activeVideoCard = null;
  let qualityProfiles = [];
  let currentVideoQuality = 'original';
  let autoQualityEnabled = true;
  let qualitySwitchLoading = false;
  let autoQualityTimer = 0;
  let lastAutoQualitySwitch = 0;
  let measuredBandwidthMbps = 0;
  let measuredBandwidthAt = 0;
  let adaptiveBandwidthFactor = 1;

  const showViewerLoader = (label = 'Cargando…') => {
    if (!viewerLoader) return;
    if (viewerLoadingText) viewerLoadingText.textContent = label;
    viewerLoader.hidden = false;
  };
  const hideViewerLoader = () => {
    if (viewerLoader) viewerLoader.hidden = true;
  };
  const readVideoPreferences = () => {
    try {
      const stored = JSON.parse(localStorage.getItem(VIDEO_PREF_KEY) || '{}');
      return {
        muted: Boolean(stored.muted),
        volume: Number.isFinite(Number(stored.volume)) ? Math.max(0, Math.min(1, Number(stored.volume))) : 1,
        playbackRate: Number.isFinite(Number(stored.playbackRate)) ? Math.max(.25, Math.min(4, Number(stored.playbackRate))) : 1
      };
    } catch (_) { return { muted: false, volume: 1, playbackRate: 1 }; }
  };
  const saveVideoPreferences = (video) => {
    try {
      localStorage.setItem(VIDEO_PREF_KEY, JSON.stringify({ muted: video.muted, volume: video.volume, playbackRate: video.playbackRate }));
    } catch (_) {}
  };
  const applyVideoPreferences = (video) => {
    const preferences = readVideoPreferences();
    video.muted = preferences.muted;
    video.volume = preferences.volume;
    video.playbackRate = preferences.playbackRate;
  };
  const formatMediaTime = (seconds) => {
    const value = Number(seconds);
    if (!Number.isFinite(value) || value < 0) return '0:00';
    const total = Math.floor(value);
    const hours = Math.floor(total / 3600);
    const minutes = Math.floor((total % 3600) / 60);
    const secs = String(total % 60).padStart(2, '0');
    return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${secs}` : `${minutes}:${secs}`;
  };
  const refreshVideoTimeline = () => {
    const video = activeVideo;
    if (!video) return;
    if (videoCurrent) videoCurrent.textContent = formatMediaTime(video.currentTime);
    if (videoDuration) videoDuration.textContent = formatMediaTime(video.duration);
    if (videoProgress && !videoScrubbing) {
      const duration = Number.isFinite(video.duration) && video.duration > 0 ? video.duration : 0;
      videoProgress.value = duration ? String((video.currentTime / duration) * 1000) : '0';
      videoProgress.disabled = !duration;
    }
  };
  const stopVideoProgressLoop = () => {
    if (videoProgressFrame) cancelAnimationFrame(videoProgressFrame);
    videoProgressFrame = 0;
  };
  const videoProgressLoop = () => {
    videoProgressFrame = 0;
    if (!activeVideo || activeVideo.paused || activeVideo.ended) return;
    refreshVideoTimeline();
    videoProgressFrame = requestAnimationFrame(videoProgressLoop);
  };
  const startVideoProgressLoop = () => {
    stopVideoProgressLoop();
    refreshVideoTimeline();
    videoProgressFrame = requestAnimationFrame(videoProgressLoop);
  };
  const refreshVideoControls = () => {
    const video = activeVideo;
    if (!video) return;
    const playing = !video.paused && !video.ended;
    if (videoPlayIcon) videoPlayIcon.hidden = playing;
    if (videoPauseIcon) videoPauseIcon.hidden = !playing;
    if (videoPlayButton) {
      videoPlayButton.title = playing ? 'Pausar' : 'Reproducir';
      videoPlayButton.setAttribute('aria-label', playing ? 'Pausar' : 'Reproducir');
    }
    refreshVideoTimeline();
    if (videoVolume) videoVolume.value = String(video.muted ? 0 : video.volume);
    if (videoSpeed) videoSpeed.value = String(video.playbackRate);
    if (videoMuteButton) {
      const muted = video.muted || video.volume === 0;
      videoMuteButton.title = muted ? 'Activar sonido' : 'Silenciar';
      videoMuteButton.setAttribute('aria-label', muted ? 'Activar sonido' : 'Silenciar');
      videoMuteButton.classList.toggle('is-muted', muted);
    }
  };

  const numericProfiles = () => qualityProfiles
    .filter((profile) => /^\d+$/.test(profile.id || ''))
    .sort((a, b) => Number(a.id) - Number(b.id));
  const browserBandwidthMbps = () => {
    const downlink = Number(connectionInfo?.downlink);
    if (Number.isFinite(downlink) && downlink > 0) return downlink;
    switch (connectionInfo?.effectiveType) {
      case 'slow-2g': return .12;
      case '2g': return .35;
      case '3g': return 1.4;
      case '4g': return 8;
      default: return 0;
    }
  };
  const effectiveBandwidthMbps = () => {
    const measuredFresh = measuredBandwidthMbps > 0 && (Date.now() - measuredBandwidthAt) < 90_000;
    const base = measuredFresh ? measuredBandwidthMbps : browserBandwidthMbps();
    return base > 0 ? base * adaptiveBandwidthFactor : 0;
  };
  const qualityBudgetMbps = { 360: .9, 480: 1.6, 720: 3.2, 1080: 5.8 };
  const chooseAutoQuality = () => {
    const profiles = numericProfiles();
    if (!profiles.length) return 'original';
    const bandwidth = effectiveBandwidthMbps();
    const stageHeight = Math.max(240, stage?.clientHeight || window.innerHeight || 720);
    const pixelRatio = Math.min(1.5, Math.max(1, window.devicePixelRatio || 1));
    const displayCap = stageHeight * pixelRatio * 1.15;
    let candidates = profiles.filter((profile) => Number(profile.id) <= displayCap);
    if (!candidates.length) candidates = [profiles[0]];
    if (bandwidth > 0) {
      const safeBudget = bandwidth * .68;
      const networkCandidates = candidates.filter((profile) => (qualityBudgetMbps[Number(profile.id)] || Infinity) <= safeBudget);
      if (networkCandidates.length) candidates = networkCandidates;
      else return profiles[0].id;
    } else {
      const fallback = candidates.filter((profile) => Number(profile.id) <= 720);
      if (fallback.length) candidates = fallback;
    }
    return candidates[candidates.length - 1]?.id || 'original';
  };
  const describeAutoQuality = (quality) => {
    const bandwidth = effectiveBandwidthMbps();
    return bandwidth > 0 ? `Auto · ${quality}p · ~${bandwidth.toFixed(1)} Mbps` : `Auto · ${quality}p`;
  };
  const measureVideoBandwidth = async (card, token) => {
    if (!card?.dataset.original || !viewer?.open || currentMediaID !== token) return;
    try {
      const controller = new AbortController();
      const started = performance.now();
      const response = await fetch(card.dataset.original, {
        headers: { Range: 'bytes=0-524287' },
        cache: 'no-store',
        signal: controller.signal
      });
      if (!response.ok || !response.body) return;
      const reader = response.body.getReader();
      let received = 0;
      while (received < 524288) {
        const { value, done } = await reader.read();
        if (done) break;
        received += value?.byteLength || 0;
      }
      await reader.cancel().catch(() => {});
      controller.abort();
      const seconds = Math.max(.03, (performance.now() - started) / 1000);
      if (received >= 64 * 1024) {
        measuredBandwidthMbps = Math.min(1000, (received * 8) / seconds / 1_000_000);
        measuredBandwidthAt = Date.now();
      }
    } catch (_) {}
  };

  const resetQualityControl = () => {
    qualityProfiles = [];
    currentVideoQuality = 'original';
    autoQualityEnabled = true;
    adaptiveBandwidthFactor = 1;
    if (qualityControl) qualityControl.hidden = true;
    if (qualityStatus) qualityStatus.textContent = '';
    if (qualitySelect) {
      qualitySelect.disabled = false;
      qualitySelect.replaceChildren(new Option('Original', 'original'));
      qualitySelect.value = 'original';
      qualitySelect.onchange = null;
    }
  };
  const replaceVideoSource = (video, url, previousTime, resumeAfter) => new Promise((resolve, reject) => {
    let settled = false;
    const cleanup = () => {
      video.removeEventListener('loadedmetadata', onMetadata);
      video.removeEventListener('canplay', onCanPlay);
      video.removeEventListener('error', onError);
      window.clearTimeout(timeout);
    };
    const finish = async () => {
      if (settled) return;
      settled = true;
      cleanup();
      applyVideoPreferences(video);
      refreshVideoControls();
      if (resumeAfter) { try { await video.play(); } catch (_) {} }
      resolve();
    };
    const onMetadata = () => {
      if (Number.isFinite(video.duration) && previousTime > 0) video.currentTime = Math.min(previousTime, Math.max(0, video.duration - .05));
      if (video.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA) finish();
    };
    const onCanPlay = () => finish();
    const onError = () => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(new Error('No se pudo cargar la resolución seleccionada.'));
    };
    const timeout = window.setTimeout(onError, 20_000);
    video.addEventListener('loadedmetadata', onMetadata);
    video.addEventListener('canplay', onCanPlay);
    video.addEventListener('error', onError);
    video.src = url;
    video.load();
  });
  const waitForVideoVariant = async (fileID, quality, token, showLoader = true) => {
    while (viewer?.open && currentMediaID === token) {
      await new Promise((resolve) => window.setTimeout(resolve, 850));
      const response = await fetch(`/api/video/${encodeURIComponent(fileID)}/estado?quality=${encodeURIComponent(quality)}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo consultar la conversión.');
      const state = await response.json();
      if (state.state === 'ready') return state;
      if (state.state === 'error') throw new Error(state.error || 'FFmpeg no pudo generar esta resolución.');
      const label = state.state === 'queued' ? `Esperando turno para ${quality}p…` : `Preparando ${quality}p…`;
      if (qualityStatus) qualityStatus.textContent = showLoader ? label : `Auto · ${label.toLowerCase()}`;
      if (showLoader) showViewerLoader(label);
    }
    throw new Error('cancelado');
  };
  const prepareVideoQuality = async (video, card, quality, mode = 'manual') => {
    const token = card.dataset.mediaId || '';
    if (!video || currentMediaID !== token || qualitySwitchLoading) return;
    if (quality === currentVideoQuality) {
      if (mode === 'auto' && qualityStatus) qualityStatus.textContent = quality === 'original' ? 'Auto · Original' : describeAutoQuality(quality);
      return;
    }
    qualitySwitchLoading = true;
    if (qualitySelect) qualitySelect.disabled = true;
    let resumeAfter = !video.paused && !video.ended;
    let previousTime = Number.isFinite(video.currentTime) ? video.currentTime : 0;
    const manualSwitch = mode !== 'auto';
    const preparingLabel = quality === 'original' ? 'Cambiando a calidad original…' : `Preparando ${quality}p…`;
    if (qualityStatus) qualityStatus.textContent = mode === 'auto' && quality !== 'original' ? `Auto · preparando ${quality}p en segundo plano…` : preparingLabel;
    if (manualSwitch) {
      stopVideoProgressLoop();
      video.pause();
      showViewerLoader(preparingLabel);
    }
    try {
      let url = card.dataset.original;
      if (quality !== 'original') {
        const response = await fetch(`/api/video/${encodeURIComponent(token)}/preparar`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8', Accept: 'application/json' },
          body: new URLSearchParams({ csrf_token: csrfToken, quality }),
          cache: 'no-store'
        });
        if (!response.ok && response.status !== 202) throw new Error((await response.text()).trim() || 'No se pudo preparar el video.');
        let state = await response.json();
        if (state.state !== 'ready') state = await waitForVideoVariant(token, quality, token, manualSwitch);
        if (currentMediaID !== token || !viewer?.open) return;
        url = state.url;
      }
      if (!manualSwitch) {
        resumeAfter = !video.paused && !video.ended;
        previousTime = Number.isFinite(video.currentTime) ? video.currentTime : 0;
        stopVideoProgressLoop();
        video.pause();
        showViewerLoader(quality === 'original' ? 'Aplicando calidad original…' : `Cambiando a ${quality}p…`);
      }
      await replaceVideoSource(video, url, previousTime, resumeAfter);
      currentVideoQuality = quality;
      lastAutoQualitySwitch = mode === 'auto' ? Date.now() : lastAutoQualitySwitch;
      if (qualityStatus) qualityStatus.textContent = mode === 'auto'
        ? (quality === 'original' ? 'Auto · Original' : describeAutoQuality(quality))
        : (quality === 'original' ? 'Original' : `${quality}p · caché local`);
    } catch (error) {
      if (error.message !== 'cancelado') {
        if (qualityStatus) qualityStatus.textContent = error.message || 'No se pudo cambiar la calidad.';
        if (manualSwitch && resumeAfter && currentMediaID === token) { try { await video.play(); } catch (_) {} }
      }
    } finally {
      if (currentMediaID === token && activeVideo === video) {
        qualitySwitchLoading = false;
        hideViewerLoader();
        if (qualitySelect) qualitySelect.disabled = false;
        refreshVideoControls();
        if (!video.paused) startVideoProgressLoop();
        if (mode === 'auto') window.setTimeout(reevaluateAutoQuality, 8200);
      }
    }
  };
  const reevaluateAutoQuality = () => {
    window.clearTimeout(autoQualityTimer);
    autoQualityTimer = window.setTimeout(() => {
      if (!autoQualityEnabled || !activeVideo || !activeVideoCard || qualitySwitchLoading || !viewer?.open) return;
      const target = chooseAutoQuality();
      if (!target || target === currentVideoQuality) {
        if (qualityStatus && target !== 'original') qualityStatus.textContent = describeAutoQuality(target);
        return;
      }
      if (Date.now() - lastAutoQualitySwitch < 8_000) return;
      if (qualitySelect) qualitySelect.value = 'auto';
      prepareVideoQuality(activeVideo, activeVideoCard, target, 'auto');
    }, 180);
  };
  const configureVideo = (video) => {
    activeVideo = video;
    applyVideoPreferences(video);
    if (videoControls) videoControls.hidden = false;
    ['loadedmetadata', 'durationchange', 'seeking', 'seeked', 'ended', 'volumechange', 'ratechange'].forEach((type) => video.addEventListener(type, refreshVideoControls));
    video.addEventListener('play', () => { refreshVideoControls(); startVideoProgressLoop(); });
    video.addEventListener('pause', () => { stopVideoProgressLoop(); refreshVideoControls(); });
    video.addEventListener('playing', () => { if (!qualitySwitchLoading) hideViewerLoader(); startVideoProgressLoop(); });
    video.addEventListener('waiting', () => {
      if (qualitySwitchLoading) return;
      showViewerLoader('Cargando video…');
      if (autoQualityEnabled) {
        adaptiveBandwidthFactor = Math.max(.42, adaptiveBandwidthFactor * .72);
        reevaluateAutoQuality();
      }
    });
    video.addEventListener('stalled', () => {
      if (autoQualityEnabled && !qualitySwitchLoading) {
        adaptiveBandwidthFactor = Math.max(.42, adaptiveBandwidthFactor * .8);
        reevaluateAutoQuality();
      }
    });
    video.addEventListener('volumechange', () => saveVideoPreferences(video));
    video.addEventListener('ratechange', () => saveVideoPreferences(video));
    video.addEventListener('click', async () => {
      if (video.paused) { try { await video.play(); } catch (_) {} } else video.pause();
    });
    video.addEventListener('dblclick', async () => {
      try {
        if (document.fullscreenElement) await document.exitFullscreen();
        else if (viewerShell?.requestFullscreen) await viewerShell.requestFullscreen();
      } catch (_) {}
    });
    refreshVideoControls();
  };
  videoPlayButton?.addEventListener('click', async () => {
    if (!activeVideo) return;
    if (activeVideo.paused) { try { await activeVideo.play(); } catch (_) {} } else activeVideo.pause();
  });
  videoProgress?.addEventListener('pointerdown', () => { videoScrubbing = true; });
  videoProgress?.addEventListener('pointerup', () => { videoScrubbing = false; refreshVideoTimeline(); });
  videoProgress?.addEventListener('change', () => { videoScrubbing = false; refreshVideoTimeline(); });
  videoProgress?.addEventListener('input', () => {
    if (!activeVideo || !Number.isFinite(activeVideo.duration) || activeVideo.duration <= 0) return;
    activeVideo.currentTime = (Number(videoProgress.value) / 1000) * activeVideo.duration;
    if (videoCurrent) videoCurrent.textContent = formatMediaTime(activeVideo.currentTime);
  });
  videoMuteButton?.addEventListener('click', () => {
    if (!activeVideo) return;
    activeVideo.muted = !activeVideo.muted;
    saveVideoPreferences(activeVideo);
    refreshVideoControls();
  });
  videoVolume?.addEventListener('input', () => {
    if (!activeVideo) return;
    activeVideo.volume = Math.max(0, Math.min(1, Number(videoVolume.value)));
    activeVideo.muted = activeVideo.volume === 0;
    saveVideoPreferences(activeVideo);
    refreshVideoControls();
  });
  videoSpeed?.addEventListener('change', () => {
    if (!activeVideo) return;
    activeVideo.playbackRate = Number(videoSpeed.value) || 1;
    saveVideoPreferences(activeVideo);
    videoSpeed.blur();
  });

  const setZoom = (value) => {
    zoom = Math.max(.5, Math.min(5, value));
    const visual = $('[data-viewer-visual]', stage);
    if (visual) visual.style.transform = `scale(${zoom})`;
  };
  const decodeViewerImage = async (src, card) => {
    if (!src) throw new Error('imagen sin origen');
    const image = new Image();
    image.className = 'viewer-image';
    image.dataset.viewerVisual = '1';
    image.dataset.downloadFileId = card.dataset.mediaId || '';
    image.alt = card.dataset.name || '';
    image.src = src;
    if (typeof image.decode === 'function') await image.decode();
    else await new Promise((resolve, reject) => { image.onload = resolve; image.onerror = reject; });
    return image;
  };
  const setupVideoQualities = async (video, card) => {
    resetQualityControl();
    activeVideoCard = card;
    const token = card.dataset.mediaId || '';
    try {
      const response = await fetch(`/api/video/${encodeURIComponent(token)}/calidades`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok || currentMediaID !== token) return;
      const data = await response.json();
      if (!data.ffmpeg || !Array.isArray(data.profiles) || data.profiles.length <= 1) return;
      qualityProfiles = data.profiles;
      qualitySelect.replaceChildren(new Option('Auto', 'auto'));
      data.profiles.forEach((profile) => {
        const option = new Option(profile.label, profile.id);
        if (profile.state === 'ready' && profile.id !== 'original') option.textContent += ' · lista';
        qualitySelect.append(option);
      });
      qualitySelect.value = 'auto';
      qualityControl.hidden = false;
      autoQualityEnabled = true;
      currentVideoQuality = 'original';
      qualityStatus.textContent = 'Auto · evaluando red…';
      qualitySelect.onchange = () => {
        const selected = qualitySelect.value;
        qualitySelect.blur();
        if (selected === 'auto') {
          autoQualityEnabled = true;
          adaptiveBandwidthFactor = 1;
          reevaluateAutoQuality();
          return;
        }
        autoQualityEnabled = false;
        prepareVideoQuality(video, card, selected, 'manual');
      };
      const staleMeasurement = !measuredBandwidthAt || Date.now() - measuredBandwidthAt > 90_000;
      if (staleMeasurement) {
        qualityStatus.textContent = 'Auto · midiendo ancho de banda…';
        await measureVideoBandwidth(card, token);
      }
      if (currentMediaID !== token || !autoQualityEnabled) return;
      const initialTarget = chooseAutoQuality();
      if (initialTarget !== 'original') prepareVideoQuality(video, card, initialTarget, 'auto');
      else qualityStatus.textContent = 'Auto · Original';
    } catch (_) {}
  };

  connectionInfo?.addEventListener?.('change', () => {
    measuredBandwidthAt = 0;
    adaptiveBandwidthFactor = 1;
    reevaluateAutoQuality();
  });
  window.addEventListener('resize', () => reevaluateAutoQuality());
  document.addEventListener('fullscreenchange', () => reevaluateAutoQuality());

  const showMedia = async (index) => {
    const cards = mediaCards();
    if (!cards.length || !viewer || !stage) return;
    currentIndex = (index + cards.length) % cards.length;
    const card = cards[currentIndex];
    $$('video,audio', stage).forEach((media) => media.pause());
    currentMediaID = card.dataset.mediaId || '';
    zoom = 1;
    stopVideoProgressLoop();
    videoScrubbing = false;
    qualitySwitchLoading = false;
    hideViewerLoader();
    resetQualityControl();
    activeVideo = null;
    activeVideoCard = null;
    if (videoControls) videoControls.hidden = true;
    viewerShell.dataset.activeKind = card.dataset.kind || '';
    $('[data-viewer-title]', viewer).textContent = card.dataset.name || '';

    if (card.dataset.kind === 'image') {
      const token = currentMediaID;
      showViewerLoader('Cargando imagen…');
      if (!viewer.open) viewer.showModal();
      try {
        const cacheIsOriented = Number(card.dataset.cacheVersion || 0) >= 2;
        const fastSource = cacheIsOriented ? (card.dataset.preview || card.dataset.thumbnail || card.dataset.original) : card.dataset.original;
        const image = await decodeViewerImage(fastSource, card);
        if (currentMediaID !== token) return;
        stage.replaceChildren(image);
        setZoom(1);
        if (cacheIsOriented && card.dataset.original && fastSource !== card.dataset.original) {
          try {
            const original = await decodeViewerImage(card.dataset.original, card);
            if (currentMediaID === token) {
              stage.replaceChildren(original);
              setZoom(zoom);
            }
          } catch (_) {}
        }
      } catch (_) {
        if (currentMediaID === token) stage.replaceChildren();
      } finally {
        if (currentMediaID === token) hideViewerLoader();
      }
    } else if (card.dataset.kind === 'video') {
      stage.replaceChildren();
      const video = document.createElement('video');
      video.className = 'viewer-video';
      video.dataset.viewerVisual = '1';
      video.dataset.downloadFileId = currentMediaID;
      video.controls = false;
      video.autoplay = true;
      video.playsInline = true;
      video.preload = 'metadata';
      video.src = card.dataset.original;
      if (card.dataset.thumbnail) video.poster = card.dataset.thumbnail;
      showViewerLoader('Cargando video…');
      configureVideo(video);
      stage.append(video);
      setupVideoQualities(video, card);
    } else {
      stage.replaceChildren();
      const wrap = document.createElement('div');
      wrap.className = 'viewer-audio';
      wrap.dataset.downloadFileId = currentMediaID;
      if (card.dataset.thumbnail) {
        const image = document.createElement('img');
        image.src = card.dataset.thumbnail;
        image.alt = '';
        wrap.append(image);
      }
      const audio = document.createElement('audio');
      audio.controls = true;
      audio.autoplay = true;
      audio.src = card.dataset.original;
      wrap.append(audio);
      stage.append(wrap);
    }
    if (!viewer.open) viewer.showModal();
  };
  grid?.addEventListener('click', (event) => {
    const card = event.target.closest('[data-media-id]');
    if (!card) return;
    if (document.body.classList.contains('selection-mode')) {
      event.preventDefault();
      toggleSelected(card);
      return;
    }
    showMedia(mediaCards().indexOf(card));
  });
  const stepMedia = async (delta) => {
    const before = mediaCards().length;
    if (delta > 0 && currentIndex === before - 1 && grid?.dataset.hasMore === 'true') await loadMoreGallery();
    await showMedia(currentIndex + delta);
  };
  $('[data-viewer-close]', viewer)?.addEventListener('click', () => viewer.close());
  $('[data-viewer-prev]', viewer)?.addEventListener('click', () => stepMedia(-1));
  $('[data-viewer-next]', viewer)?.addEventListener('click', () => stepMedia(1));
  fullscreenButton?.addEventListener('click', async () => {
    if (!activeVideo) return;
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else if (viewerShell?.requestFullscreen) await viewerShell.requestFullscreen();
      else if (typeof activeVideo.webkitEnterFullscreen === 'function') activeVideo.webkitEnterFullscreen();
    } catch (_) {}
  });
  viewer?.addEventListener('close', () => {
    $$('video,audio', stage).forEach((media) => { media.pause(); media.removeAttribute('src'); media.load(); });
    currentIndex = -1;
    currentMediaID = '';
    zoom = 1;
    stopVideoProgressLoop();
    videoScrubbing = false;
    qualitySwitchLoading = false;
    resetQualityControl();
    activeVideo = null;
    activeVideoCard = null;
    if (videoControls) videoControls.hidden = true;
    hideViewerLoader();
    if (viewerShell) viewerShell.dataset.activeKind = '';
  });
  viewer?.addEventListener('click', (event) => { if (event.target === viewer) viewer.close(); });
  const handleViewerKeydown = (event) => {
    if (!viewer?.open || event.ctrlKey || event.metaKey || event.altKey) return;
    const active = document.activeElement;
    const tag = active?.tagName;
    if (active?.isContentEditable || tag === 'TEXTAREA') return;
    if (tag === 'INPUT' && !['range', 'button'].includes((active.type || '').toLowerCase())) return;
    if (tag === 'SELECT') return;
    const key = event.key.toLowerCase();
    if (key === 'arrowleft' || key === 'a') { event.preventDefault(); event.stopPropagation(); stepMedia(-1); }
    else if (key === 'arrowright' || key === 'd') { event.preventDefault(); event.stopPropagation(); stepMedia(1); }
    else if (key === 'w') { event.preventDefault(); event.stopPropagation(); setZoom(zoom + .15); }
    else if (key === 's') { event.preventDefault(); event.stopPropagation(); setZoom(zoom - .15); }
    else if ((key === ' ' || key === 'spacebar') && activeVideo) { event.preventDefault(); event.stopPropagation(); if (activeVideo.paused) activeVideo.play().catch(() => {}); else activeVideo.pause(); }
    else if (key === 'escape') { event.preventDefault(); viewer.close(); }
  };
  // Capturar en window mantiene los atajos del visor incluso con foco en rangos de seek/volumen.
  window.addEventListener('keydown', handleViewerKeydown, true);

  // Oculta inmediatamente medios cuya unidad se desconectó; una reconexión refresca el catálogo visible.
  const refreshGalleryAvailability = async () => {
    if (!galleryPage) return;
    try {
      const response = await fetch('/api/galeria/disponibilidad', { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) return;
      const data = await response.json();
      const online = new Set(data.online_storage_ids || []);
      if (availabilityBaseline) {
        for (const id of online) {
          if (!availabilityBaseline.has(id)) {
            window.location.reload();
            return;
          }
        }
      }
      availabilityBaseline = online;
      let removed = false;
      mediaCards().forEach((card) => {
        if (!online.has(card.dataset.storageId)) {
          if (currentMediaID && card.dataset.mediaId === currentMediaID) viewer?.close();
          card.remove();
          removed = true;
        }
      });
      if (removed && grid) {
        grid.dataset.next = String(mediaCards().length);
        if (!mediaCards().length && sentinel) sentinel.textContent = 'Las unidades de estos medios están desconectadas.';
      }
    } catch (_) {}
  };
  if (galleryPage) {
    refreshGalleryAvailability();
    window.setInterval(refreshGalleryAvailability, 5000);
  }

  // Menú contextual: Galería, visor y filas de Archivos.
  document.addEventListener('contextmenu', (event) => {
    const target = event.target.closest('[data-download-file-id]');
    let fileID = target?.dataset.downloadFileId || '';
    if (!fileID && viewer?.open && viewer.contains(event.target)) fileID = currentMediaID;
    if (!fileID || target?.dataset.offline === 'true') return;
    event.preventDefault();
    showDownloadMenu(event.clientX, event.clientY, fileID);
  });

  // Listado continuo reutilizable de Archivos.
  const fileList = $('[data-files-list]');
  const fileSentinel = $('[data-files-sentinel]');
  let loadingFiles = false;
  const makeFileRow = (item) => {
    const anchor = document.createElement('a');
    anchor.className = `file-row${item.offline ? ' is-offline' : ''}${item.health === 'damaged' ? ' is-damaged' : ''}`;
    anchor.setAttribute('role', 'listitem');
    anchor.href = item.is_dir ? item.url : item.download_url;
    if (!item.is_dir && item.id) {
      anchor.dataset.downloadFileId = item.id;
      anchor.dataset.selectableFile = item.id;
      anchor.dataset.offline = String(Boolean(item.offline));
    }
    if (!item.is_dir && item.id) {
      const check = document.createElement('span');
      check.className = 'selection-check file-selection-check';
      check.setAttribute('aria-hidden', 'true');
      check.textContent = '✓';
      anchor.append(check);
    }
    const kind = document.createElement('span');
    kind.className = `file-kind${item.is_dir ? ' folder' : ''}`;
    kind.textContent = item.is_dir ? '▣' : item.kind === 'image' ? '▧' : item.kind === 'video' ? '▶' : item.kind === 'audio' ? '♪' : item.kind === 'document' ? '▤' : item.kind === 'archive' ? '▥' : '•';
    const main = document.createElement('span');
    main.className = 'file-main';
    const strong = document.createElement('strong');
    strong.textContent = item.name;
    const small = document.createElement('small');
    small.textContent = item.is_dir ? `Carpeta${item.offline ? ' · unidad desconectada' : ''}` : `${item.kind} · ${formatTime(item.mod_time)}${item.offline ? ' · original no disponible hasta reconectar' : ''}`;
    main.append(strong, small);
    const size = document.createElement('span');
    size.className = 'file-size';
    size.textContent = item.is_dir ? '—' : formatBytes(item.size);
    anchor.append(kind, main, size);
    return anchor;
  };
  const loadMoreFiles = async () => {
    if (!fileList || loadingFiles || fileList.dataset.hasMore !== 'true') return;
    loadingFiles = true;
    try {
      const query = new URLSearchParams({ path: fileList.dataset.path, offset: fileList.dataset.next || '0', limit: '100' });
      const response = await fetch(`/api/archivos/listado?${query}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) return;
      const data = await response.json();
      data.items.forEach((item) => fileList.append(makeFileRow(item)));
      fileList.dataset.next = data.next;
      fileList.dataset.hasMore = String(data.has_more);
      if (fileSentinel && !data.has_more) fileSentinel.textContent = 'Fin de la carpeta';
    } finally { loadingFiles = false; }
  };
  if (fileList?.dataset.mode === 'infinito' && fileSentinel) {
    new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) loadMoreFiles();
    }, { rootMargin: '500px' }).observe(fileSentinel);
  }

  // Selección múltiple reutilizable en Galería y Archivos.
  const selectedIDs = new Set();
  const selectionMenu = $('[data-selection-menu]');
  const bulkToolbar = $('[data-bulk-toolbar]');
  const selectionCount = $('[data-selection-count]');
  const moveDialog = $('[data-move-dialog]');
  const moveForm = $('[data-move-form]', moveDialog);
  const moveRoot = $('[data-move-root]', moveForm);
  const folderPicker = $('[data-folder-picker]', moveForm);
  const folderList = $('[data-folder-list]', moveForm);
  const folderCurrent = $('[data-folder-current]', moveForm);
  const moveTargetDir = $('[data-move-target-dir]', moveForm);
  const newFolderInput = $('[data-new-folder]', moveForm);
  let actionIDs = [];
  let longPressFired = false;
  let moveFolderPath = '';

  const renderFolderPicker = (data) => {
    moveFolderPath = data.path || '';
    if (moveTargetDir) moveTargetDir.value = moveFolderPath;
    if (folderCurrent) folderCurrent.textContent = `/${data.root || ''}${moveFolderPath ? `/${moveFolderPath}` : ''}`;
    if (!folderList) return;
    folderList.replaceChildren();
    if (!data.folders?.length) {
      const empty = document.createElement('span'); empty.className = 'muted'; empty.textContent = 'No hay subcarpetas aquí.'; folderList.append(empty); return;
    }
    data.folders.forEach((folder) => {
      const button = document.createElement('button'); button.type = 'button'; button.className = 'folder-choice';
      button.innerHTML = '<span class="folder-choice-icon">▣</span><span></span>';
      button.lastElementChild.textContent = folder.name;
      button.addEventListener('click', () => loadMoveFolders(folder.path));
      folderList.append(button);
    });
  };
  const loadMoveFolders = async (targetPath = '') => {
    if (!moveRoot?.value) { if (folderPicker) folderPicker.hidden = true; return; }
    if (folderPicker) folderPicker.hidden = false;
    if (folderList) folderList.innerHTML = '<span class="muted">Cargando carpetas…</span>';
    try {
      const query = new URLSearchParams({ root: moveRoot.value, path: targetPath });
      const response = await fetch(`/api/carpetas?${query}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'No se pudieron leer las carpetas.');
      renderFolderPicker(data);
    } catch (error) { if (folderList) folderList.textContent = error.message; }
  };

  const refreshSelectionUI = () => {
    $$('[data-selectable-file]').forEach((element) => element.classList.toggle('is-selected', selectedIDs.has(element.dataset.selectableFile)));
    if (selectionCount) selectionCount.textContent = String(selectedIDs.size);
    if (bulkToolbar) bulkToolbar.hidden = selectedIDs.size === 0;
  };
  const setSelectionMode = (enabled) => {
    document.body.classList.toggle('selection-mode', enabled);
    if (!enabled) selectedIDs.clear();
    refreshSelectionUI();
  };
  const toggleSelected = (element) => {
    const id = element?.dataset.selectableFile;
    if (!id || element.dataset.offline === 'true') return;
    if (selectedIDs.has(id)) selectedIDs.delete(id); else selectedIDs.add(id);
    refreshSelectionUI();
  };
  const hideSelectionMenu = () => { if (selectionMenu) selectionMenu.hidden = true; };
  const showSelectionMenu = (trigger) => {
    if (!selectionMenu || !trigger) return;
    const rect = trigger.getBoundingClientRect();
    selectionMenu.hidden = false;
    const width = selectionMenu.offsetWidth || 210;
    const height = selectionMenu.offsetHeight || 96;
    selectionMenu.style.left = `${Math.max(8, Math.min(window.innerWidth - width - 8, rect.right - width))}px`;
    selectionMenu.style.top = `${Math.max(8, Math.min(window.innerHeight - height - 8, rect.bottom + 8))}px`;
  };
  $$('[data-open-selection-menu]').forEach((button) => button.addEventListener('click', (event) => {
    event.stopPropagation();
    if (selectionMenu && !selectionMenu.hidden) hideSelectionMenu(); else showSelectionMenu(button);
  }));
  $('[data-selection-start]', selectionMenu)?.addEventListener('click', () => {
    hideSelectionMenu();
    setSelectionMode(true);
  });
  const selectEverythingAvailable = async () => {
    hideSelectionMenu();
    setSelectionMode(true);
    const maxItems = 500;
    // En modo continuo, completa el listado hasta el límite seguro de operaciones masivas.
    while ($$('[data-selectable-file]').filter((item) => item.dataset.offline !== 'true').length < maxItems) {
      const galleryHasMore = grid?.dataset.mode === 'infinito' && grid.dataset.hasMore === 'true';
      const filesHaveMore = fileList?.dataset.mode === 'infinito' && fileList.dataset.hasMore === 'true';
      if (!galleryHasMore && !filesHaveMore) break;
      const before = $$('[data-selectable-file]').length;
      if (galleryHasMore) await loadMoreGallery();
      else if (filesHaveMore) await loadMoreFiles();
      if ($$('[data-selectable-file]').length <= before) break;
    }
    selectedIDs.clear();
    const available = $$('[data-selectable-file]').filter((item) => item.dataset.offline !== 'true');
    available.slice(0, maxItems).forEach((item) => selectedIDs.add(item.dataset.selectableFile));
    refreshSelectionUI();
    const moreRemain = (grid?.dataset.hasMore === 'true') || (fileList?.dataset.hasMore === 'true') || available.length > maxItems;
    if (moreRemain) window.alert('Se seleccionaron los primeros 500 elementos disponibles, que es el límite seguro por operación.');
  };
  $('[data-selection-all]', selectionMenu)?.addEventListener('click', () => { selectEverythingAvailable(); });
  document.addEventListener('click', (event) => { if (!event.target.closest('[data-selection-menu]') && !event.target.closest('[data-open-selection-menu]')) hideSelectionMenu(); });
  window.addEventListener('resize', hideSelectionMenu);
  window.addEventListener('blur', hideSelectionMenu);
  $$('[data-confirm-damaged]').forEach((form) => form.addEventListener('submit', (event) => {
    if (!window.confirm('¿Eliminar permanentemente los elementos dañados de esta unidad? Esta acción no se puede deshacer.')) event.preventDefault();
  }));
  $('[data-selection-cancel]')?.addEventListener('click', () => setSelectionMode(false));
  document.addEventListener('click', (event) => {
    const row = event.target.closest('.file-row[data-selectable-file]');
    if (!row || !document.body.classList.contains('selection-mode')) return;
    event.preventDefault();
    event.stopPropagation();
    toggleSelected(row);
  });

  const selectedParams = (ids, extra = {}) => {
    const params = new URLSearchParams({ csrf_token: csrfToken, ...extra });
    ids.forEach((id) => params.append('file_id', id));
    return params;
  };
  const postAction = async (url, ids, extra = {}) => {
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8', Accept: 'application/json' },
      body: selectedParams(ids, extra), cache: 'no-store'
    });
    const contentType = response.headers.get('content-type') || '';
    const data = contentType.includes('application/json') ? await response.json() : { error: (await response.text()).trim() };
    if (!response.ok) {
      const error = new Error(data.error || 'La operación no pudo completarse.');
      error.data = data;
      throw error;
    }
    return data;
  };
  const startBatchDownload = async (ids) => {
    const data = await postAction('/api/elementos/descargar', ids);
    if (!data.url) throw new Error('El servidor no devolvió la descarga.');
    const anchor = document.createElement('a');
    anchor.href = data.url; anchor.hidden = true; document.body.append(anchor); anchor.click(); anchor.remove();
  };
  $('[data-bulk-download]')?.addEventListener('click', async () => {
    try { await startBatchDownload([...selectedIDs]); } catch (error) { window.alert(error.message); }
  });
  const openMoveDialog = (ids) => {
    actionIDs = [...ids];
    if (!actionIDs.length) return;
    hideDownloadMenu();
    if (viewer?.open) viewer.close();
    moveFolderPath = '';
    if (moveTargetDir) moveTargetDir.value = '';
    if (newFolderInput) newFolderInput.value = '';
    moveDialog?.showModal();
    if (moveRoot?.value) loadMoveFolders('');
  };
  $('[data-bulk-move]')?.addEventListener('click', () => openMoveDialog(selectedIDs));
  $$('[data-close-move]').forEach((button) => button.addEventListener('click', () => moveDialog?.close()));
  moveDialog?.addEventListener('click', (event) => { if (event.target === moveDialog) moveDialog.close(); });
  moveRoot?.addEventListener('change', () => loadMoveFolders(''));
  $('[data-folder-up]', moveForm)?.addEventListener('click', () => {
    const parts = moveFolderPath.split('/').filter(Boolean); parts.pop(); loadMoveFolders(parts.join('/'));
  });
  moveTargetDir?.addEventListener('input', () => { moveFolderPath = moveTargetDir.value.trim().replace(/^\/+|\/+$/g, ''); });
  $('[data-create-folder]', moveForm)?.addEventListener('click', async () => {
    const name = newFolderInput?.value.trim() || '';
    if (!moveRoot?.value || !name) return;
    try {
      const response = await fetch('/api/carpetas/crear', {
        method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8', Accept: 'application/json' },
        body: new URLSearchParams({ csrf_token: csrfToken, destination_root: moveRoot.value, parent: moveFolderPath, name }), cache: 'no-store'
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'No se pudo crear la carpeta.');
      if (newFolderInput) newFolderInput.value = '';
      await loadMoveFolders(data.path || moveFolderPath);
    } catch (error) { window.alert(error.message); }
  });
  moveForm?.addEventListener('submit', async (event) => {
    event.preventDefault();
    const formData = new FormData(moveForm);
    try {
      await postAction('/api/elementos/mover', actionIDs, { destination_root: formData.get('destination_root') || '', target_dir: formData.get('target_dir') || '' });
      window.location.reload();
    } catch (error) {
      window.alert(error.data?.partial ? `${error.message}\n\n${error.data.completed || 0} elemento(s) sí se movieron antes del fallo. La vista se actualizará.` : error.message);
      if (error.data?.partial) window.location.reload();
    }
  });
  const deleteIDs = async (ids) => {
    if (!ids.length || !window.confirm(`¿Eliminar permanentemente ${ids.length} elemento(s)? Esta acción no se puede deshacer.`)) return;
    await postAction('/api/elementos/eliminar', ids);
    window.location.reload();
  };
  $('[data-bulk-delete]')?.addEventListener('click', async () => {
    try { await deleteIDs([...selectedIDs]); } catch (error) { window.alert(error.message); }
  });
  $('[data-context-move]', downloadMenu)?.addEventListener('click', () => { const id = downloadTargetID; if (id) openMoveDialog([id]); });
  $('[data-context-delete]', downloadMenu)?.addEventListener('click', async () => {
    const id = downloadTargetID; hideDownloadMenu();
    try { if (id) await deleteIDs([id]); } catch (error) { window.alert(error.message); }
  });

  // Pulsación larga táctil = mismo menú de acciones que clic derecho.
  let pressTimer = 0, pressStartX = 0, pressStartY = 0;
  document.addEventListener('pointerdown', (event) => {
    if (event.pointerType !== 'touch') return;
    const target = event.target.closest('[data-download-file-id]');
    if (!target || target.dataset.offline === 'true') return;
    pressStartX = event.clientX; pressStartY = event.clientY;
    window.clearTimeout(pressTimer);
    pressTimer = window.setTimeout(() => {
      longPressFired = true;
      if (navigator.vibrate) navigator.vibrate(20);
      showDownloadMenu(pressStartX, pressStartY, target.dataset.downloadFileId);
    }, 550);
  }, { passive: true });
  document.addEventListener('pointermove', (event) => {
    if (Math.abs(event.clientX - pressStartX) > 10 || Math.abs(event.clientY - pressStartY) > 10) window.clearTimeout(pressTimer);
  }, { passive: true });
  ['pointerup', 'pointercancel'].forEach((type) => document.addEventListener(type, () => window.clearTimeout(pressTimer), { passive: true }));
  document.addEventListener('click', (event) => {
    if (!longPressFired) return;
    longPressFired = false;
    if (event.target.closest('[data-download-file-id]')) { event.preventDefault(); event.stopPropagation(); }
  }, true);
})();
