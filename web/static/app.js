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
  const fullscreenButton = $('[data-viewer-fullscreen]', viewer);
  const qualityControl = $('[data-video-quality-control]', viewer);
  const qualitySelect = $('[data-video-quality]', viewer);
  const qualityStatus = $('[data-video-quality-status]', viewer);
  let currentIndex = -1;
  let currentMediaID = '';
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
  const configureVideo = (video) => {
    applyVideoPreferences(video);
    video.addEventListener('volumechange', () => saveVideoPreferences(video));
    video.addEventListener('ratechange', () => saveVideoPreferences(video));
  };

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
  const resetQualityControl = () => {
    if (qualityControl) qualityControl.hidden = true;
    if (qualityStatus) qualityStatus.textContent = '';
    if (qualitySelect) {
      qualitySelect.disabled = false;
      qualitySelect.replaceChildren(new Option('Original', 'original'));
      qualitySelect.value = 'original';
      qualitySelect.onchange = null;
    }
  };
  const replaceVideoSource = (video, url) => new Promise((resolve, reject) => {
    const previousTime = Number.isFinite(video.currentTime) ? video.currentTime : 0;
    const wasPaused = video.paused;
    const onReady = async () => {
      video.removeEventListener('loadedmetadata', onReady);
      video.removeEventListener('error', onError);
      if (Number.isFinite(video.duration) && previousTime > 0) video.currentTime = Math.min(previousTime, Math.max(0, video.duration - .05));
      applyVideoPreferences(video);
      if (!wasPaused) { try { await video.play(); } catch (_) {} }
      resolve();
    };
    const onError = () => {
      video.removeEventListener('loadedmetadata', onReady);
      video.removeEventListener('error', onError);
      reject(new Error('No se pudo cargar la resolución seleccionada.'));
    };
    video.addEventListener('loadedmetadata', onReady);
    video.addEventListener('error', onError);
    video.src = url;
    video.load();
  });
  const waitForVideoVariant = async (fileID, quality, token) => {
    while (viewer?.open && currentMediaID === token) {
      await new Promise((resolve) => window.setTimeout(resolve, 850));
      const response = await fetch(`/api/video/${encodeURIComponent(fileID)}/estado?quality=${encodeURIComponent(quality)}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo consultar la conversión.');
      const state = await response.json();
      if (state.state === 'ready') return state;
      if (state.state === 'error') throw new Error(state.error || 'FFmpeg no pudo generar esta resolución.');
      if (qualityStatus) qualityStatus.textContent = state.state === 'queued' ? `Esperando turno para ${quality}p…` : `Preparando ${quality}p…`;
    }
    throw new Error('cancelado');
  };
  const prepareVideoQuality = async (video, card, quality) => {
    const token = card.dataset.mediaId || '';
    if (quality === 'original') {
      if (qualityStatus) qualityStatus.textContent = 'Original';
      await replaceVideoSource(video, card.dataset.original);
      return;
    }
    if (qualitySelect) qualitySelect.disabled = true;
    if (qualityStatus) qualityStatus.textContent = `Preparando ${quality}p…`;
    try {
      const response = await fetch(`/api/video/${encodeURIComponent(token)}/preparar`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8', Accept: 'application/json' },
        body: new URLSearchParams({ csrf_token: csrfToken, quality }),
        cache: 'no-store'
      });
      if (!response.ok && response.status !== 202) throw new Error((await response.text()).trim() || 'No se pudo preparar el video.');
      let state = await response.json();
      if (state.state !== 'ready') state = await waitForVideoVariant(token, quality, token);
      if (currentMediaID !== token || !viewer?.open) return;
      await replaceVideoSource(video, state.url);
      if (qualityStatus) qualityStatus.textContent = `${quality}p · caché local`;
    } catch (error) {
      if (error.message !== 'cancelado') {
        if (qualityStatus) qualityStatus.textContent = error.message || 'No se pudo cambiar la calidad.';
        if (qualitySelect) qualitySelect.value = 'original';
      }
    } finally {
      if (qualitySelect && currentMediaID === token) qualitySelect.disabled = false;
    }
  };
  const setupVideoQualities = async (video, card) => {
    resetQualityControl();
    const token = card.dataset.mediaId || '';
    try {
      const response = await fetch(`/api/video/${encodeURIComponent(token)}/calidades`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok || currentMediaID !== token) return;
      const data = await response.json();
      if (!data.ffmpeg || !Array.isArray(data.profiles) || data.profiles.length <= 1) return;
      qualitySelect.replaceChildren();
      data.profiles.forEach((profile) => {
        const option = new Option(profile.label, profile.id);
        if (profile.state === 'ready' && profile.id !== 'original') option.textContent += ' · lista';
        qualitySelect.append(option);
      });
      qualitySelect.value = 'original';
      qualityControl.hidden = false;
      qualityStatus.textContent = 'FFmpeg disponible';
      qualitySelect.onchange = () => {
        const selected = qualitySelect.value;
        qualitySelect.blur();
        prepareVideoQuality(video, card, selected);
      };
    } catch (_) {}
  };

  const showMedia = async (index) => {
    const cards = mediaCards();
    if (!cards.length || !viewer || !stage) return;
    currentIndex = (index + cards.length) % cards.length;
    const card = cards[currentIndex];
    $$('video,audio', stage).forEach((media) => media.pause());
    currentMediaID = card.dataset.mediaId || '';
    zoom = 1;
    resetQualityControl();
    viewerShell.dataset.activeKind = card.dataset.kind || '';
    $('[data-viewer-title]', viewer).textContent = card.dataset.name || '';
    fullscreenButton.hidden = card.dataset.kind !== 'video';

    if (card.dataset.kind === 'image') {
      const token = currentMediaID;
      viewerShell.classList.add('is-media-loading');
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
        if (currentMediaID === token) viewerShell.classList.remove('is-media-loading');
      }
    } else if (card.dataset.kind === 'video') {
      stage.replaceChildren();
      const video = document.createElement('video');
      video.className = 'viewer-video';
      video.dataset.viewerVisual = '1';
      video.dataset.downloadFileId = currentMediaID;
      video.controls = true;
      video.autoplay = true;
      video.playsInline = true;
      video.preload = 'metadata';
      video.src = card.dataset.original;
      if (card.dataset.thumbnail) video.poster = card.dataset.thumbnail;
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
    const video = $('video', stage);
    if (!video?.requestFullscreen) return;
    try { await video.requestFullscreen(); } catch (_) {}
  });
  viewer?.addEventListener('close', () => {
    $$('video,audio', stage).forEach((media) => { media.pause(); media.removeAttribute('src'); media.load(); });
    currentIndex = -1;
    currentMediaID = '';
    zoom = 1;
    resetQualityControl();
    viewerShell?.classList.remove('is-media-loading');
    if (viewerShell) viewerShell.dataset.activeKind = '';
  });
  viewer?.addEventListener('click', (event) => { if (event.target === viewer) viewer.close(); });
  const handleViewerKeydown = (event) => {
    if (!viewer?.open || event.ctrlKey || event.metaKey || event.altKey) return;
    const tag = document.activeElement?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
    const key = event.key.toLowerCase();
    if (key === 'arrowleft' || key === 'a') { event.preventDefault(); event.stopPropagation(); stepMedia(-1); }
    else if (key === 'arrowright' || key === 'd') { event.preventDefault(); event.stopPropagation(); stepMedia(1); }
    else if (key === 'w') { event.preventDefault(); event.stopPropagation(); setZoom(zoom + .15); }
    else if (key === 's') { event.preventDefault(); event.stopPropagation(); setZoom(zoom - .15); }
    else if (key === 'escape') { event.preventDefault(); viewer.close(); }
  };
  // Los controles nativos de <video>/<audio> pueden consumir keydown en su shadow UI.
  // Capturar en window mantiene los atajos del visor incluso después de usar seek o volumen.
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
  $$('[data-toggle-selection]').forEach((button) => button.addEventListener('click', () => setSelectionMode(!document.body.classList.contains('selection-mode'))));
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
