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
  if (indexCards.length) {
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
            if (job.state === 'scanning') detail.textContent = `${job.images || 0} imágenes · ${job.videos || 0} videos · ${job.audio || 0} audios`;
            else if (job.state === 'done') detail.textContent = 'Última indexación terminada';
            else detail.textContent = '';
          }
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
  let currentIndex = -1;
  let currentMediaID = '';
  let zoom = 1;
  let loadingGallery = false;
  let availabilityBaseline = null;

  const mediaCards = () => $$('[data-media-id]', grid || document);
  const makeMediaCard = (item) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'photo-card media-card';
    Object.assign(button.dataset, {
      mediaId: item.id,
      downloadFileId: item.id,
      storageId: item.storage_id,
      kind: item.kind,
      name: item.name,
      original: item.original_url,
      preview: item.preview_url || '',
      thumbnail: item.thumbnail_url || ''
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
    if (item.kind === 'video' || item.kind === 'audio') {
      const badge = document.createElement('span');
      badge.className = 'media-badge';
      badge.textContent = item.kind === 'video' ? 'Video' : 'Audio';
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
  const configureVideo = (video) => {
    const preferences = readVideoPreferences();
    video.muted = preferences.muted;
    video.volume = preferences.volume;
    video.playbackRate = preferences.playbackRate;
    video.addEventListener('volumechange', () => saveVideoPreferences(video));
    video.addEventListener('ratechange', () => saveVideoPreferences(video));
  };

  const setZoom = (value) => {
    zoom = Math.max(.5, Math.min(5, value));
    const visual = $('[data-viewer-visual]', stage);
    if (visual) visual.style.transform = `scale(${zoom})`;
  };
  const showMedia = (index) => {
    const cards = mediaCards();
    if (!cards.length || !viewer || !stage) return;
    currentIndex = (index + cards.length) % cards.length;
    const card = cards[currentIndex];
    currentMediaID = card.dataset.mediaId || '';
    zoom = 1;
    stage.replaceChildren();
    viewerShell.dataset.activeKind = card.dataset.kind || '';
    $('[data-viewer-title]', viewer).textContent = card.dataset.name || '';
    fullscreenButton.hidden = card.dataset.kind !== 'video';

    if (card.dataset.kind === 'image') {
      const image = document.createElement('img');
      image.className = 'viewer-image';
      image.dataset.viewerVisual = '1';
      image.dataset.downloadFileId = currentMediaID;
      image.alt = card.dataset.name || '';
      image.src = card.dataset.preview || card.dataset.thumbnail || card.dataset.original;
      stage.append(image);
      if (card.dataset.original && card.dataset.original !== image.src) {
        const original = new Image();
        original.onload = () => { if (currentMediaID === card.dataset.mediaId && stage.contains(image)) image.src = card.dataset.original; };
        original.src = card.dataset.original;
      }
    } else if (card.dataset.kind === 'video') {
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
    } else {
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
    if (card) showMedia(mediaCards().indexOf(card));
  });
  const stepMedia = async (delta) => {
    const before = mediaCards().length;
    if (delta > 0 && currentIndex === before - 1 && grid?.dataset.hasMore === 'true') await loadMoreGallery();
    showMedia(currentIndex + delta);
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
    if (viewerShell) viewerShell.dataset.activeKind = '';
  });
  viewer?.addEventListener('click', (event) => { if (event.target === viewer) viewer.close(); });
  document.addEventListener('keydown', (event) => {
    if (!viewer?.open) return;
    const tag = document.activeElement?.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
    const key = event.key.toLowerCase();
    if (key === 'arrowleft' || key === 'a') { event.preventDefault(); stepMedia(-1); }
    else if (key === 'arrowright' || key === 'd') { event.preventDefault(); stepMedia(1); }
    else if (key === 'w') { event.preventDefault(); setZoom(zoom + .15); }
    else if (key === 's') { event.preventDefault(); setZoom(zoom - .15); }
    else if (key === 'escape') viewer.close();
  });

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
    anchor.className = 'file-row';
    anchor.setAttribute('role', 'listitem');
    anchor.href = item.is_dir ? item.url : item.download_url;
    if (!item.is_dir && item.id) {
      anchor.dataset.downloadFileId = item.id;
      anchor.dataset.offline = String(Boolean(item.offline));
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
})();
