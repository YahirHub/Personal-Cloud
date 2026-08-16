(() => {
  const $ = (s, root = document) => root?.querySelector(s) || null;
  const $$ = (s, root = document) => root ? [...root.querySelectorAll(s)] : [];
  const dialog = $('[data-share-dialog]');
  if (!dialog) return;
  const form = $('[data-share-form]', dialog);
  const title = $('[data-share-title]', dialog);
  const mode = $('[data-share-access-mode]', dialog);
  const passwordRow = $('[data-share-password-row]', dialog);
  const password = $('[data-share-password]', dialog);
  const passwordHelp = $('[data-share-password-help]', dialog);
  const linkPanel = $('[data-share-link-panel]', dialog);
  const urlInput = $('[data-share-url]', dialog);
  const embedInput = $('[data-share-embed]', dialog);
  const status = $('[data-share-status]', dialog);
  const save = $('[data-share-save]', dialog);
  const renew = $('[data-share-renew]', dialog);
  const revoke = $('[data-share-revoke]', dialog);
  const csrf = $('meta[name="csrf-token"]')?.content || '';
  let fileID = '', shareID = '', shareData = null;

  const toast = (message) => {
    const node = document.createElement('div');
    node.className = 'drive-unit-toast'; node.textContent = message; document.body.append(node);
    window.setTimeout(() => node.remove(), 3200);
  };
  const fetchJSON = async (url, options = {}) => {
    const response = await fetch(url, { cache: 'no-store', ...options, headers: { Accept: 'application/json', ...(options.headers || {}) } });
    const ct = response.headers.get('content-type') || '';
    const data = ct.includes('application/json') ? await response.json() : { error: (await response.text()).trim() };
    if (!response.ok) throw new Error(data.error || 'No se pudo completar la operación.');
    return data;
  };
  const post = (url, fields = {}) => fetchJSON(url, {
    method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8' },
    body: new URLSearchParams({ csrf_token: csrf, ...fields })
  });
  const copy = async (value, label = 'Enlace copiado') => {
    if (!value) return;
    try { await navigator.clipboard.writeText(value); toast(label); }
    catch (_) { window.prompt('Copia el enlace:', value); }
  };
  const syncMode = () => { if (passwordRow) passwordRow.hidden = mode?.value !== 'password'; };
  mode?.addEventListener('change', syncMode);

  const fill = (data) => {
    shareData = data;
    shareID = data.shared ? String(data.id || '') : '';
    if (title) title.textContent = data.name ? `Compartir “${data.name}”` : 'Compartir archivo';
    if (data.shared) {
      if (mode) mode.value = data.password_protected ? 'password' : 'public';
      if (password) password.value = '';
      if (passwordHelp) passwordHelp.textContent = data.password_protected ? 'Déjala vacía para conservar la contraseña actual.' : 'Mínimo 6 caracteres.';
      if (urlInput) urlInput.value = data.url || '';
      if (embedInput) embedInput.value = data.embed_url || '';
      if (linkPanel) linkPanel.hidden = false;
      if (renew) renew.hidden = false;
      if (revoke) revoke.hidden = false;
      if (save) save.textContent = 'Guardar acceso';
      if (status) status.textContent = data.password_protected ? 'Enlace activo · protegido con contraseña' : 'Enlace activo · acceso público';
    } else {
      shareID = '';
      if (mode) mode.value = 'public';
      if (password) password.value = '';
      if (urlInput) urlInput.value = '';
      if (embedInput) embedInput.value = '';
      if (linkPanel) linkPanel.hidden = true;
      if (renew) renew.hidden = true;
      if (revoke) revoke.hidden = true;
      if (save) save.textContent = 'Crear enlace';
      if (status) status.textContent = 'Todavía no existe un enlace público.';
    }
    syncMode();
  };

  const open = async (id, existingShareID = '') => {
    fileID = String(id || '').trim();
    const requestedShareID = String(existingShareID || '').trim();
    if (!fileID && !requestedShareID) return;
    if (status) status.textContent = 'Comprobando…';
    if (!dialog.open) dialog.showModal();
    try {
      const endpoint = requestedShareID
        ? `/api/compartidos/${encodeURIComponent(requestedShareID)}`
        : `/api/archivo/${encodeURIComponent(fileID)}/compartir`;
      const data = await fetchJSON(endpoint);
      fileID = String(data.file_id || fileID);
      fill(data);
    } catch (error) { if (status) status.textContent = error.message; }
  };
  window.PersonalCloudShare = { open };

  $$('[data-share-close]', dialog).forEach((button) => button.addEventListener('click', () => dialog.close()));
  dialog.addEventListener('click', (event) => { if (event.target === dialog) dialog.close(); });
  $('[data-share-copy]', dialog)?.addEventListener('click', () => copy(urlInput?.value));
  $('[data-share-copy-embed]', dialog)?.addEventListener('click', () => copy(embedInput?.value, 'Enlace embed copiado'));

  form?.addEventListener('submit', async (event) => {
    event.preventDefault();
    if (!fileID || !mode) return;
    save.disabled = true;
    try {
      const endpoint = shareID
        ? `/api/compartidos/${encodeURIComponent(shareID)}/configurar`
        : `/api/archivo/${encodeURIComponent(fileID)}/compartir`;
      const data = await post(endpoint, { access_mode: mode.value, password: password?.value || '' });
      fill(data); toast(data.shared ? 'Configuración de uso compartido guardada' : 'Enlace creado');
      document.dispatchEvent(new CustomEvent('personalcloud:share-updated', { detail: data }));
    } catch (error) { if (status) status.textContent = error.message; }
    finally { save.disabled = false; }
  });

  renew?.addEventListener('click', async () => {
    if (!shareID) return;
    const ok = window.PersonalCloudActions?.confirmDangerousAction
      ? await window.PersonalCloudActions.confirmDangerousAction({ title: '¿Renovar este enlace?', message: 'El enlace actual dejará de funcionar inmediatamente.', detail: 'Las personas con la URL anterior perderán el acceso.', confirmLabel: 'Renovar enlace' })
      : window.confirm('¿Renovar el enlace? El anterior dejará de funcionar.');
    if (!ok) return;
    renew.disabled = true;
    try { const data = await post(`/api/compartidos/${encodeURIComponent(shareID)}/renovar`); fill(data); toast('Enlace renovado'); }
    catch (error) { if (status) status.textContent = error.message; }
    finally { renew.disabled = false; }
  });

  revoke?.addEventListener('click', async () => {
    if (!shareID) return;
    const ok = window.PersonalCloudActions?.confirmDangerousAction
      ? await window.PersonalCloudActions.confirmDangerousAction({ title: '¿Dejar de compartir?', message: 'El enlace público dejará de funcionar inmediatamente.', detail: 'El archivo original no se eliminará.', confirmLabel: 'Dejar de compartir' })
      : window.confirm('¿Dejar de compartir este archivo?');
    if (!ok) return;
    revoke.disabled = true;
    try { const removedID = shareID; await post(`/api/compartidos/${encodeURIComponent(removedID)}/eliminar`); toast('Enlace eliminado'); document.querySelector(`[data-share-row][data-share-id="${CSS.escape(removedID)}"]`)?.remove(); fill({ shared: false, file_id: fileID, name: shareData?.name || '' }); }
    catch (error) { if (status) status.textContent = error.message; }
    finally { revoke.disabled = false; }
  });

  $('[data-context-share]')?.addEventListener('click', (event) => {
    const id = event.currentTarget.dataset.fileId || '';
    document.querySelector('[data-download-menu]')?.setAttribute('hidden', '');
    open(id);
  });
  $('[data-viewer-share]')?.addEventListener('click', () => open(document.querySelector('[data-media-viewer]')?.dataset.currentFileId || ''));
  $('[data-document-viewer-share]')?.addEventListener('click', () => open(document.querySelector('[data-document-viewer]')?.dataset.currentFileId || ''));

  $$('[data-share-row-copy]').forEach((button) => button.addEventListener('click', () => copy(button.closest('[data-share-row]')?.dataset.shareUrl || '')));
  $$('[data-share-row-edit]').forEach((button) => button.addEventListener('click', () => {
    const row = button.closest('[data-share-row]');
    open(row?.dataset.fileId || '', row?.dataset.shareId || '');
  }));
  $('[data-share-delete-all]')?.addEventListener('click', async (event) => {
    const button = event.currentTarget;
    const rows = $$('[data-share-row]');
    if (!rows.length) return;
    const ok = window.PersonalCloudActions?.confirmDangerousAction
      ? await window.PersonalCloudActions.confirmDangerousAction({ title: `¿Eliminar ${rows.length} enlace(s) público(s)?`, message: 'Todos los enlaces compartidos dejarán de funcionar inmediatamente.', detail: 'Los archivos originales no se eliminarán.', confirmLabel: 'Eliminar todos' })
      : window.confirm('¿Eliminar todos los enlaces públicos?');
    if (!ok) return;
    button.disabled = true;
    try { const data = await post('/api/compartidos/eliminar-todos', { scope: button.dataset.scope || 'mine' }); toast(`${data.deleted || 0} enlace(s) eliminado(s)`); window.setTimeout(() => window.location.reload(), 250); }
    catch (error) { toast(error.message); button.disabled = false; }
  });
})();
