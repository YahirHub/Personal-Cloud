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
  if (uploadDialog && new URLSearchParams(window.location.search).get('nuevo') === '1') {
    window.setTimeout(() => uploadDialog.showModal(), 0);
  }

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
  const deleteConfirmDialog = $('[data-delete-confirm]');
  const deleteConfirmTitle = $('[data-delete-confirm-title]', deleteConfirmDialog);
  const deleteConfirmMessage = $('[data-delete-confirm-message]', deleteConfirmDialog);
  const deleteConfirmDetail = $('[data-delete-confirm-detail]', deleteConfirmDialog);
  const deleteConfirmLabel = $('[data-delete-confirm-label]', deleteConfirmDialog);
  const deleteConfirmAccept = $('[data-delete-confirm-accept]', deleteConfirmDialog);
  const deleteConfirmCancel = $('[data-delete-confirm-cancel]', deleteConfirmDialog);
  let deleteConfirmResolve = null;
  const settleDeleteConfirm = (value) => {
    const resolve = deleteConfirmResolve;
    deleteConfirmResolve = null;
    if (deleteConfirmDialog?.open) deleteConfirmDialog.close();
    if (resolve) resolve(Boolean(value));
  };
  const confirmDangerousAction = ({ title = '¿Eliminar?', message = 'El elemento se eliminará permanentemente.', detail = 'Esta acción no se puede deshacer.', confirmLabel = 'Eliminar' } = {}) => {
    if (!deleteConfirmDialog) return Promise.resolve(window.confirm(`${title}\n\n${message}\n\n${detail}`));
    if (deleteConfirmResolve) settleDeleteConfirm(false);
    if (deleteConfirmTitle) deleteConfirmTitle.textContent = title;
    if (deleteConfirmMessage) deleteConfirmMessage.textContent = message;
    if (deleteConfirmDetail) deleteConfirmDetail.textContent = detail;
    if (deleteConfirmLabel) deleteConfirmLabel.textContent = confirmLabel;
    deleteConfirmDialog.showModal();
    window.setTimeout(() => deleteConfirmCancel?.focus(), 0);
    return new Promise((resolve) => { deleteConfirmResolve = resolve; });
  };
  deleteConfirmAccept?.addEventListener('click', () => settleDeleteConfirm(true));
  deleteConfirmCancel?.addEventListener('click', () => settleDeleteConfirm(false));
  deleteConfirmDialog?.addEventListener('cancel', (event) => { event.preventDefault(); settleDeleteConfirm(false); });
  deleteConfirmDialog?.addEventListener('click', (event) => { if (event.target === deleteConfirmDialog) settleDeleteConfirm(false); });
  window.PersonalCloudConfirm = { ask: confirmDangerousAction };

  const downloadMenu = $('[data-download-menu]');
  let downloadTargetID = '';
  let downloadTargetOffline = false;
  let downloadTargetInfo = null;
  const setStarMenuState = (starred, loading = false) => {
    const button = $('[data-context-star]', downloadMenu);
    const label = $('[data-context-star-label]', downloadMenu);
    if (button) button.disabled = loading;
    if (label) label.textContent = loading ? 'Comprobando…' : (starred ? 'Quitar de Destacados' : 'Agregar a Destacados');
  };
  const loadDownloadTargetInfo = async () => {
    const id = downloadTargetID;
    if (!id) throw new Error('Archivo no seleccionado');
    if (downloadTargetInfo?.id === id) return downloadTargetInfo;
    const response = await fetch(`/api/archivo/${encodeURIComponent(id)}/info`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
    if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo obtener información');
    const info = await response.json();
    if (downloadTargetID === id) {
      downloadTargetInfo = info;
      downloadTargetOffline = info.online === false;
      setStarMenuState(Boolean(info.starred));
      $$('[data-requires-online]', downloadMenu).forEach((button) => { button.disabled = downloadTargetOffline; });
      $$(`[data-download-file-id="${CSS.escape(id)}"]`).forEach((node) => {
        node.dataset.offline = String(downloadTargetOffline);
        node.classList.toggle('is-offline', downloadTargetOffline);
      });
    }
    return info;
  };
  const updateStarStateForID = (id, starred) => {
    if (!id) return;
    const escaped = CSS.escape(id);
    $$(`[data-selectable-file="${escaped}"], [data-download-file-id="${escaped}"]`).forEach((node) => {
      node.dataset.starred = String(Boolean(starred));
      node.classList.toggle('is-starred', Boolean(starred));
    });
    if (downloadTargetInfo?.id === id) downloadTargetInfo.starred = Boolean(starred);
  };
  const requestFileStar = async (id, starred) => {
    const response = await fetch(`/api/archivo/${encodeURIComponent(id)}/destacar`, {
      method: 'POST',
      headers: { 'X-CSRF-Token': csrfToken, 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8', Accept: 'application/json' },
      body: new URLSearchParams({ starred: String(Boolean(starred)) }),
      cache: 'no-store'
    });
    const contentType = response.headers.get('content-type') || '';
    const result = contentType.includes('application/json') ? await response.json() : { error: (await response.text()).trim() };
    if (!response.ok) throw new Error(result.error || 'No se pudo actualizar Destacados');
    updateStarStateForID(id, Boolean(result.starred));
    return result;
  };
  const hideDownloadMenu = () => {
    if (!downloadMenu) return;
    downloadMenu.hidden = true;
    downloadTargetID = '';
    downloadTargetOffline = false;
    downloadTargetInfo = null;
  };
  const showDownloadMenu = (x, y, fileID, offline = false) => {
    if (!downloadMenu || !fileID) return;
    downloadTargetID = fileID;
    downloadTargetOffline = Boolean(offline);
    downloadTargetInfo = null;
    const contextShareButton = $('[data-context-share]', downloadMenu);
    if (contextShareButton) contextShareButton.dataset.fileId = fileID;
    setStarMenuState(false, true);
    $$('[data-requires-online]', downloadMenu).forEach((button) => { button.disabled = downloadTargetOffline; });
    downloadMenu.hidden = false;
    downloadMenu.style.left = '0px';
    downloadMenu.style.top = '0px';
    const rect = downloadMenu.getBoundingClientRect();
    const left = Math.max(8, Math.min(x, window.innerWidth - rect.width - 8));
    const top = Math.max(8, Math.min(y, window.innerHeight - rect.height - 8));
    downloadMenu.style.left = `${left}px`;
    downloadMenu.style.top = `${top}px`;
    loadDownloadTargetInfo().catch(() => setStarMenuState(false));
  };
  $$('[data-file-actions]').forEach((button) => button.addEventListener('click', (event) => {
    event.preventDefault();
    event.stopPropagation();
    const rect = button.getBoundingClientRect();
    const offline = button.closest('[data-offline]')?.dataset.offline === 'true';
    showDownloadMenu(rect.right - 8, rect.bottom + 6, button.dataset.fileActions, offline);
  }));

  // Menú y propiedades reales de las unidades mostradas como carpetas raíz.
  const unitMenu = $('[data-unit-menu]');
  const unitDialog = $('[data-unit-dialog]');
  let unitTargetID = '';
  let unitTargetURL = '';
  let unitInfoCache = null;
  const hideUnitMenu = () => { if (unitMenu) unitMenu.hidden = true; };
  const showToast = (message) => {
    const toast = document.createElement('div');
    toast.className = 'drive-unit-toast';
    toast.textContent = message;
    document.body.append(toast);
    window.setTimeout(() => toast.remove(), 3200);
  };

  // Menú Nuevo estilo Drive: crear carpeta o subir, siempre conectado a backend real.
  const globalNewButton = $('[data-global-new]');
  const newMenu = $('[data-new-menu]');
  const folderDialog = $('[data-new-folder-dialog]');
  const folderForm = $('[data-new-folder-form]', folderDialog);
  const positionNewMenu = () => {
    if (!newMenu || !globalNewButton) return;
    const buttonRect = globalNewButton.getBoundingClientRect();
    newMenu.hidden = false;
    newMenu.style.left = '0px';
    newMenu.style.top = '0px';
    const rect = newMenu.getBoundingClientRect();
    const left = Math.max(8, Math.min(buttonRect.left, window.innerWidth - rect.width - 8));
    const top = Math.max(8, Math.min(buttonRect.bottom + 8, window.innerHeight - rect.height - 8));
    newMenu.style.left = `${left}px`;
    newMenu.style.top = `${top}px`;
    globalNewButton.setAttribute('aria-expanded', 'true');
  };
  const hideNewMenu = () => {
    if (newMenu) newMenu.hidden = true;
    globalNewButton?.setAttribute('aria-expanded', 'false');
  };
  globalNewButton?.addEventListener('click', (event) => {
    event.preventDefault();
    event.stopPropagation();
    if (!newMenu) { window.location.assign('/archivos?nuevo=1'); return; }
    if (newMenu.hidden) positionNewMenu(); else hideNewMenu();
  });
  $('[data-new-upload]', newMenu)?.addEventListener('click', () => {
    hideNewMenu();
    if (uploadDialog) uploadDialog.showModal(); else window.location.assign('/archivos?nuevo=1');
  });
  const openNewFolderDialog = () => {
    hideNewMenu();
    if (!folderDialog) { window.location.assign('/archivos?carpeta=1'); return; }
    folderDialog.showModal();
    const input = $('input[name="name"]', folderDialog);
    if (input) { input.value = ''; window.setTimeout(() => input.focus(), 0); }
  };
  $('[data-new-folder]', newMenu)?.addEventListener('click', openNewFolderDialog);
  $('[data-close-new-folder]')?.addEventListener('click', () => folderDialog?.close());
  $('[data-cancel-new-folder]')?.addEventListener('click', () => folderDialog?.close());
  folderDialog?.addEventListener('click', (event) => { if (event.target === folderDialog) folderDialog.close(); });
  if (folderDialog && new URLSearchParams(window.location.search).get('carpeta') === '1') {
    window.setTimeout(() => openNewFolderDialog(), 0);
  }
  folderForm?.addEventListener('submit', async (event) => {
    event.preventDefault();
    const form = new FormData(folderForm);
    const root = String(form.get('destination_root') || '').trim();
    const parent = String(form.get('parent') || '').trim();
    const name = String(form.get('name') || '').trim();
    if (!root || !name) return;
    const submit = $('button[type="submit"]', folderForm);
    if (submit) submit.disabled = true;
    try {
      const response = await fetch('/api/carpetas/crear', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8', Accept: 'application/json' },
        body: new URLSearchParams({ csrf_token: csrfToken, destination_root: root, parent, name }),
        cache: 'no-store'
      });
      if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo crear la carpeta');
      const data = await response.json();
      folderDialog.close();
      showToast('Carpeta creada');
      const parts = [root, ...(String(data.path || '').split('/').filter(Boolean))].map(encodeURIComponent);
      window.setTimeout(() => window.location.assign(`/archivos/ver/${parts.join('/')}`), 160);
    } catch (error) { showToast(error.message || 'No se pudo crear la carpeta'); }
    finally { if (submit) submit.disabled = false; }
  });
  document.addEventListener('click', (event) => { if (!event.target.closest('[data-new-menu]') && !event.target.closest('[data-global-new]')) hideNewMenu(); });

  // Selector explícito de destino para Subir: Automático sigue siendo el valor
  // predeterminado, pero el usuario puede elegir unidad y navegar carpetas.
  const uploadLocationPanel = $('[data-upload-location-panel]', uploadDialog);
  const uploadRoot = $('[data-upload-root]', uploadDialog);
  const uploadTargetDir = $('[data-upload-target-dir]', uploadDialog);
  const uploadFolderPicker = $('[data-upload-folder-picker]', uploadDialog);
  const uploadFolderList = $('[data-upload-folder-list]', uploadDialog);
  const uploadFolderCurrent = $('[data-upload-folder-current]', uploadDialog);
  const uploadDestinationLabel = $('[data-upload-destination-label]', uploadDialog);
  let uploadFolderPath = '';
  const syncUploadDestinationLabel = () => {
    if (!uploadDestinationLabel) return;
    const root = String(uploadRoot?.value || '').trim();
    const dir = String(uploadTargetDir?.value || '').trim().replace(/^\/+|\/+$/g, '');
    if (!root) {
      const currentDrop = $('[data-drive-files-page]');
      uploadDestinationLabel.textContent = currentDrop?.dataset.currentPath && currentDrop.dataset.currentPath !== '/'
        ? currentDrop.dataset.currentPath
        : 'Automático · el sistema decide';
      return;
    }
    uploadDestinationLabel.textContent = `/${root}${dir ? `/${dir}` : ''}`;
  };
  const loadUploadFolders = async (folderPath = '') => {
    const root = String(uploadRoot?.value || '').trim();
    uploadFolderPath = String(folderPath || '').replace(/^\/+|\/+$/g, '');
    if (uploadTargetDir) uploadTargetDir.value = uploadFolderPath;
    syncUploadDestinationLabel();
    if (!root || !uploadFolderList || !uploadFolderPicker) {
      if (uploadFolderPicker) uploadFolderPicker.hidden = true;
      return;
    }
    uploadFolderPicker.hidden = false;
    if (uploadFolderCurrent) uploadFolderCurrent.textContent = `/${root}${uploadFolderPath ? `/${uploadFolderPath}` : ''}`;
    uploadFolderList.innerHTML = '<span class="muted">Cargando carpetas…</span>';
    try {
      const params = new URLSearchParams({ root, path: uploadFolderPath });
      const response = await fetch(`/api/carpetas?${params}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'No se pudieron leer las carpetas.');
      uploadFolderList.replaceChildren();
      if (!data.folders?.length) {
        const empty = document.createElement('span'); empty.className = 'muted'; empty.textContent = 'No hay subcarpetas.'; uploadFolderList.append(empty);
      } else {
        data.folders.forEach((folder) => {
          const button = document.createElement('button');
          button.type = 'button'; button.className = 'folder-list-item';
          const icon = document.createElement('span'); icon.className = 'folder-list-icon'; icon.textContent = '📁';
          const label = document.createElement('span'); label.textContent = folder.name;
          button.append(icon, label);
          button.addEventListener('click', () => loadUploadFolders(folder.path));
          uploadFolderList.append(button);
        });
      }
    } catch (error) {
      uploadFolderList.replaceChildren();
      const failed = document.createElement('span'); failed.className = 'muted'; failed.textContent = error.message || 'No se pudieron cargar las carpetas.'; uploadFolderList.append(failed);
    }
  };
  $('[data-upload-choose-location]', uploadDialog)?.addEventListener('click', () => {
    if (!uploadLocationPanel) return;
    uploadLocationPanel.hidden = !uploadLocationPanel.hidden;
    if (!uploadLocationPanel.hidden && uploadRoot?.value) loadUploadFolders(uploadTargetDir?.value || '');
  });
  uploadRoot?.addEventListener('change', () => {
    uploadFolderPath = '';
    if (uploadTargetDir) uploadTargetDir.value = '';
    loadUploadFolders('');
    syncUploadDestinationLabel();
  });
  uploadTargetDir?.addEventListener('input', () => { uploadFolderPath = uploadTargetDir.value.trim().replace(/^\/+|\/+$/g, ''); syncUploadDestinationLabel(); });
  $('[data-upload-folder-up]', uploadDialog)?.addEventListener('click', () => {
    const parts = uploadFolderPath.split('/').filter(Boolean); parts.pop(); loadUploadFolders(parts.join('/'));
  });
  syncUploadDestinationLabel();

  // Arrastrar y soltar archivos sobre Mi unidad o una carpeta, como en Drive.
  const filesPageDrop = $('[data-drive-files-page]');
  const dropOverlay = $('[data-drop-overlay]');
  if (filesPageDrop?.dataset.uploadDrop === 'true') {
    let dragDepth = 0;
    const showDrop = () => { if (dropOverlay) dropOverlay.hidden = false; filesPageDrop.classList.add('is-dragging-files'); };
    const hideDrop = () => { dragDepth = 0; if (dropOverlay) dropOverlay.hidden = true; filesPageDrop.classList.remove('is-dragging-files'); };
    filesPageDrop.addEventListener('dragenter', (event) => {
      if (!event.dataTransfer?.types?.includes('Files')) return;
      event.preventDefault(); dragDepth++; showDrop();
    });
    filesPageDrop.addEventListener('dragover', (event) => {
      if (!event.dataTransfer?.types?.includes('Files')) return;
      event.preventDefault(); event.dataTransfer.dropEffect = 'copy'; showDrop();
    });
    filesPageDrop.addEventListener('dragleave', (event) => {
      event.preventDefault(); dragDepth = Math.max(0, dragDepth - 1); if (!dragDepth) hideDrop();
    });
    filesPageDrop.addEventListener('drop', async (event) => {
      event.preventDefault();
      const files = [...(event.dataTransfer?.files || [])].filter((file) => file && file.name);
      hideDrop();
      if (!files.length) return;
      const currentPath = filesPageDrop.dataset.currentPath || '/';
      let uploaded = 0;
      for (const file of files) {
        const body = new FormData();
        body.append('csrf_token', csrfToken);
        body.append('current_path', currentPath);
        body.append('target_dir', '');
        body.append('file', file, file.name);
        try {
          showToast(`Subiendo ${uploaded + 1} de ${files.length}: ${file.name}`);
          const response = await fetch('/archivos/subir', { method: 'POST', body, redirect: 'follow' });
          const finalURL = new URL(response.url, window.location.origin);
          const uploadError = finalURL.searchParams.get('error');
          if (!response.ok || uploadError) throw new Error(uploadError || `No se pudo subir ${file.name}`);
          uploaded++;
        } catch (error) { showToast(error.message || `No se pudo subir ${file.name}`); break; }
      }
      if (uploaded) { showToast(`${uploaded} archivo${uploaded === 1 ? '' : 's'} subido${uploaded === 1 ? '' : 's'}`); window.setTimeout(() => window.location.reload(), 420); }
    });
  }
  const positionUnitMenu = (x, y) => {
    if (!unitMenu) return;
    unitMenu.hidden = false;
    unitMenu.style.left = '0px';
    unitMenu.style.top = '0px';
    const rect = unitMenu.getBoundingClientRect();
    unitMenu.style.left = `${Math.max(8, Math.min(x, window.innerWidth - rect.width - 8))}px`;
    unitMenu.style.top = `${Math.max(8, Math.min(y, window.innerHeight - rect.height - 8))}px`;
  };
  const loadUnitInfo = async () => {
    if (!unitTargetID) throw new Error('Unidad no seleccionada');
    const response = await fetch(`/api/almacenamiento/${encodeURIComponent(unitTargetID)}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
    if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo leer la unidad');
    unitInfoCache = await response.json();
    const mountButton = $('[data-unit-mount]', unitMenu);
    if (mountButton) {
      mountButton.hidden = Boolean(unitInfoCache.mounted);
      mountButton.disabled = !unitInfoCache.online;
      mountButton.title = unitInfoCache.online ? '' : 'La unidad no está presente físicamente';
    }
    return unitInfoCache;
  };
  const showUnitMenu = (x, y, id, url) => {
    if (!unitMenu || !id) return;
    unitTargetID = id;
    unitTargetURL = url || '';
    unitInfoCache = null;
    hideDownloadMenu();
    positionUnitMenu(x, y);
    loadUnitInfo().catch(() => {});
  };
  $$('[data-unit-actions]').forEach((button) => button.addEventListener('click', (event) => {
    event.preventDefault();
    event.stopPropagation();
    const rect = button.getBoundingClientRect();
    showUnitMenu(rect.right - 8, rect.bottom + 6, button.dataset.unitActions, button.dataset.unitUrl);
  }));
  $('[data-unit-open]', unitMenu)?.addEventListener('click', () => {
    const target = unitTargetURL;
    hideUnitMenu();
    if (target) window.location.assign(target);
  });
  const fillUnitDialog = (info) => {
    if (!unitDialog || !info) return;
    const title = $('[data-unit-dialog-title]', unitDialog);
    if (title) title.textContent = info.virtual_root || info.name || 'Unidad';
    const status = $('[data-unit-dialog-status]', unitDialog);
    if (status) {
      status.classList.toggle('online', Boolean(info.online));
      status.classList.toggle('offline', !info.online);
      status.textContent = info.online
        ? `${info.status || 'Conectada'}${info.mounted ? ' · lista para servir originales' : ' · se montará al abrir un archivo'}`
        : 'Unidad desconectada · el catálogo local continúa disponible';
    }
    $$('[data-unit-field]', unitDialog).forEach((node) => {
      const value = info[node.dataset.unitField];
      node.textContent = value === null || value === undefined || value === '' ? '—' : String(value);
    });
    $$('[data-unit-bytes]', unitDialog).forEach((node) => {
      node.textContent = formatBytes(info[node.dataset.unitBytes] || 0);
    });
    const mode = $('[data-unit-mode]', unitDialog);
    if (mode) mode.textContent = info.read_only ? 'Solo lectura' : 'Lectura y escritura';
    const indexState = $('[data-unit-index-state]', unitDialog);
    if (indexState) {
      const labels = { queued: 'En cola', counting: 'Contando', scanning: 'Indexando', done: 'Actualizado', error: 'Error' };
      indexState.textContent = `${labels[info.index_state] || 'Sin actividad'}${['queued','counting','scanning'].includes(info.index_state) ? ` · ${info.index_percent || 0}%` : ''}${info.index_error ? ` · ${info.index_error}` : ''}`;
    }
  };
  $('[data-unit-info]', unitMenu)?.addEventListener('click', async () => {
    hideUnitMenu();
    if (!unitDialog) return;
    unitDialog.showModal();
    try { fillUnitDialog(unitInfoCache || await loadUnitInfo()); }
    catch (error) {
      const status = $('[data-unit-dialog-status]', unitDialog);
      if (status) { status.className = 'drive-properties-status offline'; status.textContent = error.message || 'No se pudo obtener información'; }
    }
  });
  $('[data-unit-dialog-close]')?.addEventListener('click', () => unitDialog?.close());
  unitDialog?.addEventListener('click', (event) => { if (event.target === unitDialog) unitDialog.close(); });
  $('[data-unit-index]', unitMenu)?.addEventListener('click', async () => {
    const id = unitTargetID;
    hideUnitMenu();
    if (!id) return;
    try {
      const response = await fetch(`/api/almacenamiento/${encodeURIComponent(id)}/indexar`, { method: 'POST', headers: { 'X-CSRF-Token': csrfToken, Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo actualizar el catálogo');
      showToast('Actualización del catálogo iniciada');
    } catch (error) { showToast(error.message || 'No se pudo actualizar el catálogo'); }
  });
  $('[data-unit-mount]', unitMenu)?.addEventListener('click', async () => {
    const id = unitTargetID;
    hideUnitMenu();
    if (!id) return;
    try {
      const response = await fetch(`/api/almacenamiento/${encodeURIComponent(id)}/montar`, { method: 'POST', headers: { 'X-CSRF-Token': csrfToken, Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo conectar la unidad');
      unitInfoCache = null;
      showToast('Unidad conectada y lista para servir archivos');
      window.setTimeout(() => window.location.reload(), 450);
    } catch (error) { showToast(error.message || 'No se pudo conectar la unidad'); }
  });
  document.addEventListener('click', (event) => { if (!event.target.closest('[data-unit-menu]') && !event.target.closest('[data-unit-actions]')) hideUnitMenu(); });
  document.addEventListener('contextmenu', (event) => {
    const card = event.target.closest('.drive-folder-unit,.file-root-card');
    if (!card) return;
    const button = $('[data-unit-actions]', card);
    if (!button) return;
    event.preventDefault();
    showUnitMenu(event.clientX, event.clientY, button.dataset.unitActions, button.dataset.unitUrl);
  });

  const applyViewMode = (container, buttons, mode, storageKey) => {
    if (!container || !['grid', 'list'].includes(mode)) return;
    container.classList.toggle('is-grid', mode === 'grid');
    container.classList.toggle('is-list', mode === 'list');
    buttons.forEach((button) => button.classList.toggle('active', button.dataset.fileView === mode || button.dataset.homeView === mode));
    try { window.localStorage.setItem(storageKey, mode); } catch (_) {}
  };
  const fileViewContainer = $('[data-files-list]');
  const fileViewButtons = $$('[data-file-view]');
  if (fileViewContainer && fileViewButtons.length) {
    let mode = 'grid';
    try { mode = window.localStorage.getItem('pc-drive-file-view') || 'grid'; } catch (_) {}
    applyViewMode(fileViewContainer, fileViewButtons, mode, 'pc-drive-file-view');
    fileViewButtons.forEach((button) => button.addEventListener('click', () => applyViewMode(fileViewContainer, fileViewButtons, button.dataset.fileView, 'pc-drive-file-view')));
  }
  const homeViewContainer = $('[data-home-files]');
  const homeViewButtons = $$('[data-home-view]');
  if (homeViewContainer && homeViewButtons.length) {
    let mode = 'grid';
    try { mode = window.localStorage.getItem('pc-drive-home-view') || 'grid'; } catch (_) {}
    applyViewMode(homeViewContainer, homeViewButtons, mode, 'pc-drive-home-view');
    homeViewButtons.forEach((button) => button.addEventListener('click', () => applyViewMode(homeViewContainer, homeViewButtons, button.dataset.homeView, 'pc-drive-home-view')));
  }
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
  $('[data-context-open]', downloadMenu)?.addEventListener('click', async () => {
    const id = downloadTargetID;
    hideDownloadMenu();
    if (!id) return;
    try {
      const mediaHandled = window.PersonalCloudMediaViewer ? await window.PersonalCloudMediaViewer.open(id) : false;
      if (mediaHandled) return;
      const documentHandled = window.PersonalCloudDocumentViewer ? await window.PersonalCloudDocumentViewer.open(id) : false;
      if (!documentHandled) window.location.assign(`/archivo/${encodeURIComponent(id)}/original`);
    } catch (error) {
      showToast(error.message || 'No se pudo abrir el archivo');
    }
  });
  const fileInfoDialog = $('[data-file-info-dialog]');
  $('[data-context-info]', downloadMenu)?.addEventListener('click', async () => {
    const id = downloadTargetID;
    hideDownloadMenu();
    if (!id || !fileInfoDialog) return;
    fileInfoDialog.showModal();
    const status = $('[data-file-info-status]', fileInfoDialog);
    if (status) { status.className = 'drive-properties-status'; status.textContent = 'Cargando…'; }
    try {
      const response = await fetch(`/api/archivo/${encodeURIComponent(id)}/info`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo obtener información');
      const info = await response.json();
      const title = $('[data-file-info-title]', fileInfoDialog);
      if (title) title.textContent = info.name || 'Archivo';
      if (status) {
        status.classList.toggle('online', Boolean(info.online));
        status.classList.toggle('offline', !info.online);
        status.textContent = info.online ? 'Original disponible' : 'Original temporalmente no disponible · metadata conservada en el catálogo';
      }
      $$('[data-file-info-field]', fileInfoDialog).forEach((node) => {
        const value = info[node.dataset.fileInfoField];
        node.textContent = value === null || value === undefined || value === '' ? '—' : String(value);
      });
      $$('[data-file-info-bytes]', fileInfoDialog).forEach((node) => { node.textContent = formatBytes(info[node.dataset.fileInfoBytes] || 0); });
      $$('[data-file-info-time]', fileInfoDialog).forEach((node) => { node.textContent = formatTime(info[node.dataset.fileInfoTime]); });
      const resolution = $('[data-file-info-resolution]', fileInfoDialog);
      if (resolution) resolution.textContent = info.width && info.height ? `${info.width} × ${info.height}` : '—';
      const starred = $('[data-file-info-starred]', fileInfoDialog);
      if (starred) starred.textContent = info.starred ? 'Sí' : 'No';
    } catch (error) {
      if (status) { status.className = 'drive-properties-status offline'; status.textContent = error.message || 'No se pudo obtener información'; }
    }
  });
  $('[data-file-info-close]')?.addEventListener('click', () => fileInfoDialog?.close());
  fileInfoDialog?.addEventListener('click', (event) => { if (event.target === fileInfoDialog) fileInfoDialog.close(); });

  $('[data-context-star]', downloadMenu)?.addEventListener('click', async () => {
    const id = downloadTargetID;
    if (!id) return;
    try {
      const info = downloadTargetInfo || await loadDownloadTargetInfo();
      const result = await requestFileStar(id, !Boolean(info.starred));
      if (downloadTargetInfo) downloadTargetInfo.starred = Boolean(result.starred);
      setStarMenuState(Boolean(result.starred));
      hideDownloadMenu();
      showToast(result.starred ? 'Agregado a Destacados' : 'Quitado de Destacados');
      if (window.location.pathname === '/destacados' && !result.starred) window.setTimeout(() => window.location.reload(), 220);
    } catch (error) {
      setStarMenuState(Boolean(downloadTargetInfo?.starred));
      showToast(error.message || 'No se pudo actualizar Destacados');
    }
  });

  const renameDialog = $('[data-rename-dialog]');
  const renameForm = $('[data-rename-form]', renameDialog);
  const renameInput = $('[data-rename-input]', renameDialog);
  let renameTargetID = '';
  const closeRenameDialog = () => { renameTargetID = ''; renameDialog?.close(); };
  $('[data-context-rename]', downloadMenu)?.addEventListener('click', async () => {
    const id = downloadTargetID;
    if (!id || !renameDialog || !renameInput) return;
    try {
      const info = downloadTargetInfo || await loadDownloadTargetInfo();
      renameTargetID = id;
      renameInput.value = info.name || '';
      hideDownloadMenu();
      renameDialog.showModal();
      window.setTimeout(() => { renameInput.focus(); renameInput.select(); }, 0);
    } catch (error) { showToast(error.message || 'No se pudo preparar el renombrado'); }
  });
  $('[data-rename-close]')?.addEventListener('click', closeRenameDialog);
  $('[data-rename-cancel]')?.addEventListener('click', closeRenameDialog);
  renameDialog?.addEventListener('click', (event) => { if (event.target === renameDialog) closeRenameDialog(); });
  renameForm?.addEventListener('submit', async (event) => {
    event.preventDefault();
    const id = renameTargetID;
    const name = renameInput?.value.trim() || '';
    if (!id || !name) return;
    const submit = renameForm.querySelector('button[type="submit"]');
    if (submit) submit.disabled = true;
    try {
      const response = await fetch(`/api/archivo/${encodeURIComponent(id)}/renombrar`, {
        method: 'POST',
        headers: { 'X-CSRF-Token': csrfToken, 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8', Accept: 'application/json' },
        body: new URLSearchParams({ name }),
        cache: 'no-store'
      });
      if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo renombrar');
      closeRenameDialog();
      showToast('Archivo renombrado');
      window.setTimeout(() => window.location.reload(), 180);
    } catch (error) { showToast(error.message || 'No se pudo renombrar el archivo'); }
    finally { if (submit) submit.disabled = false; }
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
  const viewerStarButton = $('[data-viewer-star]', viewer);
  const viewerDeleteButton = $('[data-viewer-delete]', viewer);
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
  const viewerPrevButton = $('[data-viewer-prev]', viewer);
  const viewerNextButton = $('[data-viewer-next]', viewer);
  const qualityControl = $('[data-video-quality-control]', viewer);
  const qualitySelect = $('[data-video-quality]', viewer);
  const qualityStatus = $('[data-video-quality-status]', viewer);
  let currentIndex = -1;
  let currentMediaID = '';
  let activeVideo = null;
  let activeMediaCard = null;
  let viewerNavigationMode = 'none';
  let pageMediaIDs = [];
  let pageMediaIndex = -1;
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
      cacheVersion: String(item.cache_version || 0),
      starred: String(Boolean(item.starred))
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
  let qualitySwitchSequence = 0;
  let pendingVideoQuality = '';
  let initialVideoLoading = false;
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
    qualitySwitchSequence += 1;
    pendingVideoQuality = '';
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
  const waitForMediaEvent = (media, events, timeoutMs = 20_000) => new Promise((resolve, reject) => {
    let settled = false;
    const names = Array.isArray(events) ? events : [events];
    const cleanup = () => {
      names.forEach((name) => media.removeEventListener(name, onReady));
      media.removeEventListener('error', onError);
      window.clearTimeout(timeout);
    };
    const onReady = () => {
      if (settled) return;
      settled = true;
      cleanup();
      resolve();
    };
    const onError = () => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(new Error('No se pudo cargar la resolución seleccionada.'));
    };
    const timeout = window.setTimeout(onError, timeoutMs);
    names.forEach((name) => media.addEventListener(name, onReady, { once: true }));
    media.addEventListener('error', onError, { once: true });
  });
  const waitForVideoVariant = async (fileID, quality, token, requestID, mode) => {
    while (viewer?.open && currentMediaID === token && requestID === qualitySwitchSequence) {
      await new Promise((resolve) => window.setTimeout(resolve, 850));
      const response = await fetch(`/api/video/${encodeURIComponent(fileID)}/estado?quality=${encodeURIComponent(quality)}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo consultar la conversión.');
      const state = await response.json();
      if (state.state === 'ready') return state;
      if (state.state === 'error') throw new Error(state.error || 'FFmpeg no pudo generar esta resolución.');
      if (qualityStatus && requestID === qualitySwitchSequence) {
        qualityStatus.textContent = mode === 'auto' ? `Auto · preparando ${quality}p…` : `${quality}p · preparando en segundo plano…`;
      }
    }
    throw new Error('cancelado');
  };
  const stageVideoSourceSwap = async (current, url, card, token, requestID) => {
    if (!current || !stage || currentMediaID !== token || requestID !== qualitySwitchSequence) throw new Error('cancelado');
    const resumeAfter = !current.paused && !current.ended;
    const preferences = { muted: current.muted, volume: current.volume, playbackRate: current.playbackRate };
    const staged = document.createElement('video');
    staged.className = 'viewer-video viewer-video-staging';
    staged.dataset.viewerVisual = '1';
    staged.dataset.downloadFileId = token;
    staged.controls = false;
    staged.autoplay = false;
    staged.playsInline = true;
    staged.preload = 'auto';
    staged.muted = true;
    staged.volume = 0;
    staged.playbackRate = preferences.playbackRate;
    staged.src = url;
    if (card?.dataset.thumbnail) staged.poster = card.dataset.thumbnail;
    stage.append(staged);
    const cleanupStaged = () => {
      staged.pause();
      staged.removeAttribute('src');
      staged.load();
      staged.remove();
    };
    try {
      if (staged.readyState < HTMLMediaElement.HAVE_METADATA) await waitForMediaEvent(staged, 'loadedmetadata');
      if (currentMediaID !== token || requestID !== qualitySwitchSequence || activeVideo !== current) throw new Error('cancelado');
      const syncTime = Number.isFinite(current.currentTime) ? current.currentTime : 0;
      if (syncTime > 0 && Number.isFinite(staged.duration)) {
        const target = Math.min(syncTime, Math.max(0, staged.duration - .05));
        if (Math.abs(staged.currentTime - target) > .08) {
          staged.currentTime = target;
          await waitForMediaEvent(staged, 'seeked');
        }
      }
      if (staged.readyState < HTMLMediaElement.HAVE_FUTURE_DATA) await waitForMediaEvent(staged, 'canplay');
      if (currentMediaID !== token || requestID !== qualitySwitchSequence || activeVideo !== current) throw new Error('cancelado');
      if (resumeAfter) {
        // El video activo continúa reproduciéndose mientras la nueva calidad se prepara.
        // La copia de staging arranca silenciada y fuera de vista para hacer el swap sin pausa visible.
        const liveTime = Number.isFinite(current.currentTime) ? current.currentTime : syncTime;
        if (Number.isFinite(staged.duration) && Math.abs(staged.currentTime - liveTime) > .35) {
          staged.currentTime = Math.min(liveTime, Math.max(0, staged.duration - .05));
          await waitForMediaEvent(staged, 'seeked');
        }
        await staged.play();
      }
      if (currentMediaID !== token || requestID !== qualitySwitchSequence || activeVideo !== current) throw new Error('cancelado');
      current.pause();
      current.remove();
      staged.classList.remove('viewer-video-staging');
      staged.muted = preferences.muted;
      staged.volume = preferences.volume;
      staged.playbackRate = preferences.playbackRate;
      configureVideo(staged, { initial: initialVideoLoading });
      if (!resumeAfter) staged.pause();
      return staged;
    } catch (error) {
      cleanupStaged();
      throw error;
    }
  };
  const prepareVideoQuality = async (quality, mode = 'manual') => {
    const video = activeVideo;
    const card = activeVideoCard;
    const token = card?.dataset.mediaId || '';
    if (!video || !card || currentMediaID !== token) return;
    if (quality === currentVideoQuality && !pendingVideoQuality) {
      if (qualityStatus) qualityStatus.textContent = mode === 'auto'
        ? (quality === 'original' ? 'Auto · Original' : describeAutoQuality(quality))
        : (quality === 'original' ? 'Original' : `${quality}p`);
      return;
    }
    if (mode === 'auto' && pendingVideoQuality) return;
    const requestID = ++qualitySwitchSequence;
    pendingVideoQuality = quality;
    if (qualityStatus) qualityStatus.textContent = quality === 'original'
      ? (mode === 'auto' ? 'Auto · Original solicitada' : 'Original · solicitada')
      : (mode === 'auto' ? `Auto · solicitando ${quality}p…` : `${quality}p · solicitada`);
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
        if (state.state !== 'ready') state = await waitForVideoVariant(token, quality, token, requestID, mode);
        if (currentMediaID !== token || requestID !== qualitySwitchSequence || !viewer?.open) return;
        url = state.url;
      }
      await stageVideoSourceSwap(video, url, card, token, requestID);
      if (currentMediaID !== token || requestID !== qualitySwitchSequence) return;
      currentVideoQuality = quality;
      if (mode === 'auto') lastAutoQualitySwitch = Date.now();
      if (qualityStatus) qualityStatus.textContent = mode === 'auto'
        ? (quality === 'original' ? 'Auto · Original' : describeAutoQuality(quality))
        : (quality === 'original' ? 'Original' : `${quality}p`);
    } catch (error) {
      if (error.message !== 'cancelado' && requestID === qualitySwitchSequence && qualityStatus) {
        qualityStatus.textContent = error.message || 'No se pudo cambiar la calidad.';
      }
    } finally {
      if (requestID === qualitySwitchSequence) {
        pendingVideoQuality = '';
        if (mode === 'auto') window.setTimeout(reevaluateAutoQuality, 8200);
      }
    }
  };
  const reevaluateAutoQuality = () => {
    window.clearTimeout(autoQualityTimer);
    autoQualityTimer = window.setTimeout(() => {
      if (!autoQualityEnabled || !activeVideo || !activeVideoCard || pendingVideoQuality || !viewer?.open) return;
      const target = chooseAutoQuality();
      if (!target || target === currentVideoQuality) {
        if (qualityStatus) qualityStatus.textContent = target === 'original' ? 'Auto · Original' : describeAutoQuality(target);
        return;
      }
      if (Date.now() - lastAutoQualitySwitch < 8_000) return;
      if (qualitySelect) qualitySelect.value = 'auto';
      prepareVideoQuality(target, 'auto');
    }, 180);
  };
  const configureVideo = (video, options = {}) => {
    activeVideo = video;
    if (options.initial) initialVideoLoading = true;
    applyVideoPreferences(video);
    if (videoControls) videoControls.hidden = false;
    ['loadedmetadata', 'durationchange', 'seeking', 'seeked', 'ended', 'volumechange', 'ratechange'].forEach((type) => video.addEventListener(type, refreshVideoControls));
    video.addEventListener('play', () => { refreshVideoControls(); startVideoProgressLoop(); });
    video.addEventListener('pause', () => { stopVideoProgressLoop(); refreshVideoControls(); });
    const finishInitialVideoLoad = () => {
      if (activeVideo !== video || !initialVideoLoading) return;
      initialVideoLoading = false;
      hideViewerLoader();
    };
    video.addEventListener('canplay', finishInitialVideoLoad);
    video.addEventListener('playing', () => { finishInitialVideoLoad(); startVideoProgressLoop(); });
    video.addEventListener('waiting', () => {
      if (initialVideoLoading && activeVideo === video) showViewerLoader('Cargando video…');
      if (autoQualityEnabled) {
        adaptiveBandwidthFactor = Math.max(.42, adaptiveBandwidthFactor * .72);
        reevaluateAutoQuality();
      }
    });
    video.addEventListener('stalled', () => {
      if (autoQualityEnabled) {
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
    if (initialVideoLoading && video.readyState >= HTMLMediaElement.HAVE_FUTURE_DATA) finishInitialVideoLoad();
    if (!video.paused && !video.ended) startVideoProgressLoop();
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
          qualitySwitchSequence += 1;
          pendingVideoQuality = '';
          reevaluateAutoQuality();
          return;
        }
        autoQualityEnabled = false;
        prepareVideoQuality(selected, 'manual');
      };
      const staleMeasurement = !measuredBandwidthAt || Date.now() - measuredBandwidthAt > 90_000;
      if (staleMeasurement) {
        qualityStatus.textContent = 'Auto · midiendo ancho de banda…';
        await measureVideoBandwidth(card, token);
      }
      if (currentMediaID !== token || !autoQualityEnabled) return;
      const initialTarget = chooseAutoQuality();
      if (initialTarget !== 'original') prepareVideoQuality(initialTarget, 'auto');
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

  const setViewerStarState = (starred, loading = false) => {
    if (!viewerStarButton) return;
    const value = Boolean(starred);
    viewerStarButton.dataset.starred = String(value);
    viewerStarButton.classList.toggle('is-starred', value);
    viewerStarButton.disabled = loading;
    const label = value ? 'Quitar de Destacados' : 'Agregar a Destacados';
    viewerStarButton.setAttribute('aria-label', loading ? 'Actualizando Destacados' : label);
    viewerStarButton.title = loading ? 'Actualizando…' : label;
  };

  const mediaViewerKinds = new Set(['image', 'video', 'audio']);
  const refreshViewerNavigation = () => {
    if (!viewerPrevButton || !viewerNextButton) return;
    let count = 0;
    if (viewerNavigationMode === 'gallery') count = mediaCards().length;
    else if (viewerNavigationMode === 'page') count = pageMediaIDs.length;
    const hide = count <= 1;
    viewerPrevButton.hidden = hide;
    viewerNextButton.hidden = hide;
  };
  const showMediaCard = async (card) => {
    if (!card || !viewer || !stage || !mediaViewerKinds.has(card.dataset.kind || '')) return false;
    $$('video,audio', stage).forEach((media) => media.pause());
    currentMediaID = card.dataset.mediaId || '';
    viewer.dataset.currentFileId = currentMediaID;
    activeMediaCard = card;
    zoom = 1;
    stopVideoProgressLoop();
    videoScrubbing = false;
    qualitySwitchSequence += 1;
    pendingVideoQuality = '';
    initialVideoLoading = false;
    hideViewerLoader();
    resetQualityControl();
    activeVideo = null;
    activeVideoCard = null;
    if (videoControls) videoControls.hidden = true;
    viewerShell.dataset.activeKind = card.dataset.kind || '';
    $('[data-viewer-title]', viewer).textContent = card.dataset.name || '';
    setViewerStarState(card.dataset.starred === 'true');
    refreshViewerNavigation();

    if (card.dataset.kind === 'image') {
      const token = currentMediaID;
      showViewerLoader('Cargando imagen…');
      if (!viewer.open) viewer.showModal();
      try {
        const cacheIsOriented = Number(card.dataset.cacheVersion || 0) >= 2;
        const sources = cacheIsOriented
          ? [card.dataset.preview, card.dataset.thumbnail, card.dataset.original]
          : [card.dataset.original, card.dataset.preview, card.dataset.thumbnail];
        let image = null;
        let sourceUsed = '';
        for (const source of [...new Set(sources.filter(Boolean))]) {
          try {
            image = await decodeViewerImage(source, card);
            sourceUsed = source;
            break;
          } catch (_) {}
        }
        if (!image) throw new Error('imagen no compatible');
        if (currentMediaID !== token) return true;
        stage.replaceChildren(image);
        setZoom(1);
        // Si la caché ya aplica EXIF y el navegador entiende el original,
        // sustituimos la preview por el original. Si no entiende HEIC/RAW/etc.,
        // se conserva la preview local en vez de dejar el visor en blanco.
        if (cacheIsOriented && card.dataset.original && sourceUsed !== card.dataset.original) {
          try {
            const original = await decodeViewerImage(card.dataset.original, card);
            if (currentMediaID === token) {
              stage.replaceChildren(original);
              setZoom(zoom);
            }
          } catch (_) {}
        }
      } catch (_) {
        if (currentMediaID === token) {
          const message = document.createElement('div');
          message.className = 'viewer-media-error';
          message.textContent = 'No se pudo mostrar esta imagen en el navegador.';
          stage.replaceChildren(message);
        }
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
      configureVideo(video, { initial: true });
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
      audio.preload = 'metadata';
      audio.src = card.dataset.original;
      wrap.append(audio);
      stage.append(wrap);
    }
    if (!viewer.open) viewer.showModal();
    return true;
  };
  const showMedia = async (index) => {
    const cards = mediaCards();
    if (!cards.length) return false;
    viewerNavigationMode = 'gallery';
    currentIndex = (index + cards.length) % cards.length;
    pageMediaIDs = [];
    pageMediaIndex = -1;
    return showMediaCard(cards[currentIndex]);
  };
  const mediaCardFromInfo = (info) => {
    const card = document.createElement('div');
    Object.assign(card.dataset, {
      mediaId: String(info.id || ''),
      kind: String(info.viewer || info.kind || ''),
      name: String(info.name || ''),
      original: String(info.original_url || `/archivo/${encodeURIComponent(info.id || '')}/original`),
      preview: String(info.preview_url || ''),
      thumbnail: String(info.thumbnail_url || ''),
      cacheVersion: String(info.cache_version || 0),
      starred: String(Boolean(info.starred))
    });
    return card;
  };
  const collectPageMediaIDs = () => {
    const seen = new Set();
    const ids = [];
    $$('[data-download-file-id][data-viewer]').forEach((node) => {
      if (!mediaViewerKinds.has(node.dataset.viewer || '') || node.dataset.offline === 'true') return;
      const id = node.dataset.downloadFileId || '';
      if (!id || seen.has(id)) return;
      seen.add(id);
      ids.push(id);
    });
    return ids;
  };
  const openMediaByID = async (fileID, options = {}) => {
    const id = String(fileID || '').trim();
    if (!id) return false;
    const response = await fetch(`/api/archivo/${encodeURIComponent(id)}/info`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
    if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo obtener información del archivo.');
    const info = await response.json();
    if (!mediaViewerKinds.has(String(info.viewer || ''))) return false;
    if (info.online === false) throw new Error('La unidad que contiene este archivo no está conectada.');
    if (!options.preserveNavigation) {
      viewerNavigationMode = 'page';
      pageMediaIDs = collectPageMediaIDs();
      if (!pageMediaIDs.includes(id)) pageMediaIDs = [id, ...pageMediaIDs];
    }
    pageMediaIndex = pageMediaIDs.indexOf(id);
    currentIndex = -1;
    const card = mediaCardFromInfo(info);
    return showMediaCard(card);
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
    if (viewerNavigationMode === 'gallery') {
      const before = mediaCards().length;
      if (delta > 0 && currentIndex === before - 1 && grid?.dataset.hasMore === 'true') await loadMoreGallery();
      await showMedia(currentIndex + delta);
      return;
    }
    if (viewerNavigationMode !== 'page' || pageMediaIDs.length <= 1) return;
    pageMediaIndex = (pageMediaIndex + delta + pageMediaIDs.length) % pageMediaIDs.length;
    await openMediaByID(pageMediaIDs[pageMediaIndex], { preserveNavigation: true });
  };
  viewerStarButton?.addEventListener('click', async () => {
    const id = currentMediaID;
    if (!id) return;
    const card = activeMediaCard || mediaCards().find((item) => item.dataset.mediaId === id);
    const current = card?.dataset.starred === 'true';
    setViewerStarState(current, true);
    try {
      const result = await requestFileStar(id, !current);
      if (card) card.dataset.starred = String(Boolean(result.starred));
      setViewerStarState(Boolean(result.starred));
      showToast(result.starred ? 'Agregado a Destacados' : 'Quitado de Destacados');
    } catch (error) {
      setViewerStarState(current);
      showToast(error.message || 'No se pudo actualizar Destacados');
    }
  });
  viewerDeleteButton?.addEventListener('click', async () => {
    const id = currentMediaID;
    if (!id) return;
    viewerDeleteButton.disabled = true;
    try {
      const deleted = await deleteIDs([id], { reload: false, source: 'viewer' });
      if (deleted) {
        if (viewer?.open) viewer.close();
        window.setTimeout(() => window.location.reload(), 180);
      }
    } catch (error) { showToast(error.message || 'No se pudo eliminar el archivo'); }
    finally { viewerDeleteButton.disabled = false; }
  });
  $('[data-viewer-close]', viewer)?.addEventListener('click', () => viewer.close());
  viewerPrevButton?.addEventListener('click', () => stepMedia(-1));
  viewerNextButton?.addEventListener('click', () => stepMedia(1));
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
    delete viewer.dataset.currentFileId;
    zoom = 1;
    stopVideoProgressLoop();
    videoScrubbing = false;
    qualitySwitchSequence += 1;
    pendingVideoQuality = '';
    initialVideoLoading = false;
    resetQualityControl();
    activeVideo = null;
    activeVideoCard = null;
    activeMediaCard = null;
    viewerNavigationMode = 'none';
    pageMediaIDs = [];
    pageMediaIndex = -1;
    if (videoControls) videoControls.hidden = true;
    refreshViewerNavigation();
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
    else if (key === 'f' && viewerStarButton) { event.preventDefault(); event.stopPropagation(); viewerStarButton.click(); }
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

  // Reutiliza el mismo visor multimedia desde Mi unidad, Inicio, búsqueda,
  // Recientes, Destacados y cualquier listado que declare data-viewer.
  document.addEventListener('click', (event) => {
    if (event.defaultPrevented || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    if (document.body.classList.contains('selection-mode') || event.target.closest('button')) return;
    const carrier = event.target.closest('[data-download-file-id][data-viewer]');
    if (!carrier || !mediaViewerKinds.has(carrier.dataset.viewer || '')) return;
    const id = carrier.dataset.downloadFileId || '';
    if (!id) return;
    event.preventDefault();
    if (carrier.dataset.offline === 'true') {
      showToast('La unidad de este archivo no está conectada');
      return;
    }
    openMediaByID(id).catch((error) => showToast(error.message || 'No se pudo abrir el archivo'));
  });
  window.PersonalCloudMediaViewer = { open: openMediaByID };

  // Menú contextual: Galería, visor y filas de Archivos.
  document.addEventListener('contextmenu', (event) => {
    const target = event.target.closest('[data-download-file-id]');
    const selectableID = target?.dataset.selectableFile || '';
    if (document.body.classList.contains('selection-mode') && selectableID && target?.dataset.offline !== 'true') {
      event.preventDefault();
      if (!selectedIDs.has(selectableID)) {
        selectedIDs.clear();
        selectedIDs.add(selectableID);
        refreshSelectionUI();
      }
      showSelectionContext(event.clientX, event.clientY);
      return;
    }
    let fileID = target?.dataset.downloadFileId || '';
    if (!fileID && viewer?.open && viewer.contains(event.target)) fileID = currentMediaID;
    if (!fileID) return;
    event.preventDefault();
    hideSelectionContext();
    showDownloadMenu(event.clientX, event.clientY, fileID, target?.dataset.offline === 'true');
  });

  // Listado continuo reutilizable de Archivos.
  const fileFilterForm = $('[data-file-filter-form]');
  $$('[data-file-filter]', fileFilterForm || document).forEach((select) => select.addEventListener('change', () => {
    const page = fileFilterForm?.querySelector('[name=pagina]');
    if (page) page.remove();
    fileFilterForm?.requestSubmit();
  }));
  $('[data-clear-file-filters]')?.addEventListener('click', () => {
    const params = new URLSearchParams(window.location.search);
    ['tipo', 'modificado', 'fuente', 'pagina'].forEach((key) => params.delete(key));
    const query = params.toString();
    window.location.assign(`${window.location.pathname}${query ? `?${query}` : ''}`);
  });

  const fileList = $('[data-files-list]');
  const folderListing = $('[data-folder-listing]');
  const folderSection = $('[data-folder-section]');
  const fileSection = $('[data-file-section]');
  const fileSentinel = $('[data-files-sentinel]');
  let loadingFiles = false;
  const localFileIconAssets = new Set(['android', 'pdf', 'markdown', 'word', 'excel', 'powerpoint', 'image', 'audio', 'video', 'database', 'archive', 'executable', 'document']);
  const makeFileTypeIcon = (item) => {
    const key = String(item.icon_key || 'file').toLowerCase().replace(/[^a-z0-9_-]/g, '') || 'file';
    const icon = document.createElement('span');
    icon.className = `file-type-icon file-type-${key}`;
    icon.setAttribute('aria-hidden', 'true');
    if (localFileIconAssets.has(key)) {
      const image = document.createElement('img');
      image.className = 'file-type-vendored';
      image.src = `/static/icons/${key}.svg`;
      image.alt = '';
      icon.append(image);
      return icon;
    }
    icon.innerHTML = '<svg viewBox="0 0 28 34" focusable="false"><path class="file-type-sheet" d="M5 1.5h11.5L23 8v24.5H5z"></path><path class="file-type-corner" d="M16.5 1.5V8H23"></path></svg>';
    const badge = document.createElement('span');
    badge.className = 'file-type-badge';
    badge.textContent = item.icon_label || 'FILE';
    icon.append(badge);
    return icon;
  };
  const makeFolderCard = (item) => {
    const anchor = document.createElement('a');
    anchor.className = `drive-folder-card${item.offline ? ' is-offline' : ''}`;
    anchor.href = item.url;
    anchor.dataset.offline = String(Boolean(item.offline));
    const icon = document.createElement('span');
    icon.className = 'drive-folder-card-icon';
    icon.innerHTML = '<svg class="inline-icon" viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6.5A2.5 2.5 0 0 1 5.5 4H10l2 2h6.5A2.5 2.5 0 0 1 21 8.5v9A2.5 2.5 0 0 1 18.5 20h-13A2.5 2.5 0 0 1 3 17.5z"></path></svg>';
    const strong = document.createElement('strong');
    strong.textContent = item.name;
    anchor.append(icon, strong);
    return anchor;
  };
  const makeFileRow = (item) => {
    const anchor = document.createElement('a');
    anchor.className = `file-row${item.offline ? ' is-offline' : ''}${item.health === 'damaged' ? ' is-damaged' : ''}`;
    anchor.setAttribute('role', 'listitem');
    anchor.href = item.download_url;
    if (item.id) {
      anchor.dataset.downloadFileId = item.id;
      anchor.dataset.selectableFile = item.id;
      anchor.dataset.offline = String(Boolean(item.offline));
      anchor.dataset.starred = String(Boolean(item.starred));
      if (item.viewer) anchor.dataset.viewer = item.viewer;
      if (item.viewer) anchor.dataset.editable = String(Boolean(item.editable));
    }
    const check = document.createElement('span');
    check.className = 'selection-check file-selection-check';
    check.setAttribute('aria-hidden', 'true');
    check.textContent = '✓';
    anchor.append(check);

    const preview = document.createElement('span');
    preview.className = 'file-card-preview';
    if (item.thumbnail_url) {
      const image = document.createElement('img');
      image.src = item.thumbnail_url;
      image.alt = '';
      image.loading = 'lazy';
      preview.append(image);
    } else {
      const placeholder = document.createElement('span');
      placeholder.className = `drive-preview-placeholder drive-kind-${item.kind || 'file'}`;
      placeholder.append(makeFileTypeIcon(item));
      preview.append(placeholder);
    }
    const kind = document.createElement('span');
    kind.className = `file-kind drive-kind-${item.kind || 'file'}`;
    kind.append(makeFileTypeIcon(item));
    const main = document.createElement('span');
    main.className = 'file-main';
    const strong = document.createElement('strong');
    strong.textContent = item.name;
    const small = document.createElement('small');
    small.textContent = `${item.location ? `${item.location} · ` : ''}${formatTime(item.mod_time)}${item.offline ? ' · original no disponible hasta reconectar' : ''}`;
    main.append(strong, small);
    const size = document.createElement('span');
    size.className = 'file-size';
    size.textContent = formatBytes(item.size);
    anchor.append(preview, kind, main, size);
    if (item.id) {
      const more = document.createElement('button');
      more.type = 'button';
      more.className = 'drive-more-button file-row-more';
      more.dataset.fileActions = item.id;
      more.setAttribute('aria-label', `Más acciones para ${item.name}`);
      more.textContent = '⋮';
      more.addEventListener('click', (event) => {
        event.preventDefault();
        event.stopPropagation();
        const rect = more.getBoundingClientRect();
        showDownloadMenu(rect.right - 8, rect.bottom + 6, item.id, Boolean(item.offline));
      });
      anchor.append(more);
    }
    return anchor;
  };
  const loadMoreFiles = async () => {
    if (!fileList || loadingFiles || fileList.dataset.hasMore !== 'true') return;
    loadingFiles = true;
    try {
      const query = new URLSearchParams({ path: fileList.dataset.path, offset: fileList.dataset.next || '0', limit: '100' });
      const currentParams = new URLSearchParams(window.location.search);
      ['tipo', 'modificado', 'fuente'].forEach((key) => {
        const value = currentParams.get(key);
        if (value) query.set(key, value);
      });
      const response = await fetch(`/api/archivos/listado?${query}`, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) return;
      const data = await response.json();
      data.items.forEach((item) => {
        if (item.is_dir && folderListing) {
          folderListing.append(makeFolderCard(item));
          if (folderSection) folderSection.hidden = false;
        } else if (!item.is_dir) {
          fileList.append(makeFileRow(item));
          if (fileSection) fileSection.hidden = false;
        }
      });
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
  const bulkStarButton = $('[data-bulk-star]');
  const bulkShareButton = $('[data-bulk-share]');
  const bulkStarLabel = $('[data-bulk-star-label]', bulkStarButton);
  const bulkDownloadButton = $('[data-bulk-download]');
  const bulkMoveButton = $('[data-bulk-move]');
  const bulkDeleteButton = $('[data-bulk-delete]');
  const selectionContext = $('[data-selection-context]');
  const selectionContextStar = $('[data-selection-context-star]', selectionContext);
  const selectionContextStarLabel = $('[data-selection-context-star-label]', selectionContext);
  const selectionContextShare = $('[data-selection-context-share]', selectionContext);
  const selectionContextDownload = $('[data-selection-context-download]', selectionContext);
  const selectionContextMove = $('[data-selection-context-move]', selectionContext);
  const selectionContextDelete = $('[data-selection-context-delete]', selectionContext);
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
    const selectable = $$('[data-selectable-file]');
    selectable.forEach((element) => element.classList.toggle('is-selected', selectedIDs.has(element.dataset.selectableFile)));
    if (selectionCount) selectionCount.textContent = String(selectedIDs.size);
    if (bulkToolbar) bulkToolbar.hidden = selectedIDs.size === 0;
    if (bulkShareButton) bulkShareButton.hidden = selectedIDs.size !== 1;
    if (bulkStarButton) {
      const allStarred = selectedIDs.size > 0 && [...selectedIDs].every((id) => selectable.some((element) => element.dataset.selectableFile === id && element.dataset.starred === 'true'));
      bulkStarButton.dataset.starred = String(allStarred);
      bulkStarButton.classList.toggle('is-starred', allStarred);
      bulkStarButton.title = allStarred ? 'Quitar de Destacados' : 'Agregar a Destacados';
      if (bulkStarLabel) bulkStarLabel.textContent = allStarred ? 'Quitar de Destacados' : 'Agregar a Destacados';
    }
    syncSelectionContext();
  };
  const setSelectionMode = (enabled) => {
    document.body.classList.toggle('selection-mode', enabled);
    if (!enabled) { selectedIDs.clear(); hideSelectionContext(); }
    refreshSelectionUI();
  };
  const toggleSelected = (element) => {
    const id = element?.dataset.selectableFile;
    if (!id || element.dataset.offline === 'true') return;
    if (selectedIDs.has(id)) selectedIDs.delete(id); else selectedIDs.add(id);
    refreshSelectionUI();
  };
  const hideSelectionContext = () => { if (selectionContext) selectionContext.hidden = true; };
  const syncSelectionContext = () => {
    if (!selectionContext) return;
    const selectable = $$('[data-selectable-file]');
    const allStarred = selectedIDs.size > 0 && [...selectedIDs].every((id) => selectable.some((element) => element.dataset.selectableFile === id && element.dataset.starred === 'true'));
    if (selectionContextStarLabel) selectionContextStarLabel.textContent = allStarred ? 'Quitar de Destacados' : 'Agregar a Destacados';
    if (selectionContextStar) selectionContextStar.dataset.starred = String(allStarred);
    if (selectionContextShare) selectionContextShare.hidden = selectedIDs.size !== 1;
  };
  const showSelectionContext = (x, y) => {
    if (!selectionContext || selectedIDs.size === 0) return;
    hideDownloadMenu();
    syncSelectionContext();
    selectionContext.hidden = false;
    selectionContext.style.left = '0px';
    selectionContext.style.top = '0px';
    const rect = selectionContext.getBoundingClientRect();
    selectionContext.style.left = `${Math.max(8, Math.min(x, window.innerWidth - rect.width - 8))}px`;
    selectionContext.style.top = `${Math.max(8, Math.min(y, window.innerHeight - rect.height - 8))}px`;
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
  document.addEventListener('click', (event) => {
    if (!event.target.closest('[data-selection-menu]') && !event.target.closest('[data-open-selection-menu]')) hideSelectionMenu();
    if (!event.target.closest('[data-selection-context]')) hideSelectionContext();
  });
  window.addEventListener('resize', () => { hideSelectionMenu(); hideSelectionContext(); });
  window.addEventListener('blur', () => { hideSelectionMenu(); hideSelectionContext(); });
  $$('[data-confirm-damaged]').forEach((form) => form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const confirmed = await confirmDangerousAction({
      title: '¿Eliminar los elementos dañados?',
      message: 'Se eliminarán permanentemente de esta unidad los archivos multimedia marcados como dañados.',
      detail: 'Esta acción no se puede deshacer.',
      confirmLabel: 'Eliminar dañados'
    });
    if (confirmed) form.submit();
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
  bulkShareButton?.addEventListener('click', () => {
    if (selectedIDs.size !== 1) return;
    const id = [...selectedIDs][0];
    window.PersonalCloudShare?.open?.(id);
  });
  bulkStarButton?.addEventListener('click', async () => {
    const ids = [...selectedIDs];
    if (!ids.length) return;
    const target = bulkStarButton.dataset.starred !== 'true';
    bulkStarButton.disabled = true;
    try {
      const result = await postAction('/api/elementos/destacar', ids, { starred: String(target) });
      ids.forEach((id) => updateStarStateForID(id, Boolean(result.starred)));
      showToast(result.starred ? `${result.updated || ids.length} agregado(s) a Destacados` : `${result.updated || ids.length} quitado(s) de Destacados`);
      refreshSelectionUI();
      if (window.location.pathname === '/destacados' && !result.starred) window.setTimeout(() => window.location.reload(), 220);
    } catch (error) {
      window.alert(error.message);
    } finally {
      bulkStarButton.disabled = false;
    }
  });
    bulkDownloadButton?.addEventListener('click', async () => {
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
  bulkMoveButton?.addEventListener('click', () => openMoveDialog(selectedIDs));
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
  const deleteIDs = async (ids, options = {}) => {
    const unique = [...new Set((ids || []).filter(Boolean))];
    if (!unique.length) return false;
    const singleName = unique.length === 1
      ? ($(`[data-download-file-id="${CSS.escape(unique[0])}"]`)?.querySelector('strong')?.textContent || downloadTargetInfo?.name || '')
      : '';
    const confirmed = await confirmDangerousAction({
      title: unique.length === 1 ? '¿Eliminar este archivo?' : `¿Eliminar ${unique.length} elementos?`,
      message: singleName ? `“${singleName}” se eliminará permanentemente del medio.` : `${unique.length} elementos se eliminarán permanentemente de sus unidades.`,
      detail: 'Esta acción elimina los originales y no se puede deshacer.',
      confirmLabel: unique.length === 1 ? 'Eliminar' : `Eliminar ${unique.length}`
    });
    if (!confirmed) return false;
    try {
      await postAction('/api/elementos/eliminar', unique);
    } catch (error) {
      if (error.data?.partial) {
        showToast(`${error.data.completed || 0} elemento(s) se eliminaron antes del fallo; actualizando la vista…`);
        window.setTimeout(() => window.location.reload(), 700);
      }
      throw error;
    }
    unique.forEach((id) => {
      const escaped = CSS.escape(id);
      $$(`[data-download-file-id="${escaped}"], [data-selectable-file="${escaped}"]`).forEach((node) => node.remove());
      selectedIDs.delete(id);
    });
    refreshSelectionUI();
    showToast(unique.length === 1 ? 'Archivo eliminado' : `${unique.length} elementos eliminados`);
    if (options.reload !== false) window.setTimeout(() => window.location.reload(), 160);
    return true;
  };
  bulkDeleteButton?.addEventListener('click', async () => {
    try { await deleteIDs([...selectedIDs]); } catch (error) { window.alert(error.message); }
  });
  selectionContextStar?.addEventListener('click', () => { hideSelectionContext(); bulkStarButton?.click(); });
  selectionContextShare?.addEventListener('click', () => { hideSelectionContext(); bulkShareButton?.click(); });
  selectionContextDownload?.addEventListener('click', () => { hideSelectionContext(); bulkDownloadButton?.click(); });
  selectionContextMove?.addEventListener('click', () => { hideSelectionContext(); bulkMoveButton?.click(); });
  selectionContextDelete?.addEventListener('click', () => { hideSelectionContext(); bulkDeleteButton?.click(); });
  $('[data-context-move]', downloadMenu)?.addEventListener('click', () => { const id = downloadTargetID; if (id) openMoveDialog([id]); });
  $('[data-context-delete]', downloadMenu)?.addEventListener('click', async () => {
    const id = downloadTargetID; hideDownloadMenu();
    try { if (id) await deleteIDs([id]); } catch (error) { window.alert(error.message); }
  });
  window.PersonalCloudActions = {
    showFileContext: (x, y, fileID, offline = false) => showDownloadMenu(x, y, fileID, offline),
    deleteFiles: (ids, options = {}) => deleteIDs(ids, options),
    confirmDangerousAction
  };

  // Pulsación larga táctil = mismo menú de acciones que clic derecho.
  let pressTimer = 0, pressStartX = 0, pressStartY = 0;
  document.addEventListener('pointerdown', (event) => {
    if (event.pointerType !== 'touch') return;
    const target = event.target.closest('[data-download-file-id]');
    if (!target) return;
    pressStartX = event.clientX; pressStartY = event.clientY;
    window.clearTimeout(pressTimer);
    pressTimer = window.setTimeout(() => {
      longPressFired = true;
      if (navigator.vibrate) navigator.vibrate(20);
      const selectableID = target.dataset.selectableFile || '';
      if (document.body.classList.contains('selection-mode') && selectableID && target.dataset.offline !== 'true') {
        if (!selectedIDs.has(selectableID)) {
          selectedIDs.clear();
          selectedIDs.add(selectableID);
          refreshSelectionUI();
        }
        showSelectionContext(pressStartX, pressStartY);
      } else {
        showDownloadMenu(pressStartX, pressStartY, target.dataset.downloadFileId, target.dataset.offline === 'true');
      }
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
