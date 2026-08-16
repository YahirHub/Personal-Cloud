(() => {
  const $ = (selector, root = document) => root?.querySelector(selector) || null;
  const $$ = (selector, root = document) => root ? [...root.querySelectorAll(selector)] : [];
  const dialog = $('[data-document-viewer]');
  if (!dialog) return;

  const title = $('[data-document-viewer-title]', dialog);
  const status = $('[data-document-viewer-status]', dialog);
  const typeBadge = $('[data-document-viewer-type]', dialog);
  const tabs = $('[data-document-viewer-tabs]', dialog);
  const previewTab = $('[data-document-viewer-preview-tab]', dialog);
  const sourceTab = $('[data-document-viewer-source-tab]', dialog);
  const starButton = $('[data-document-viewer-star]', dialog);
  const editButton = $('[data-document-viewer-edit]', dialog);
  const saveButton = $('[data-document-viewer-save]', dialog);
  const cancelButton = $('[data-document-viewer-cancel]', dialog);
  const downloadButton = $('[data-document-viewer-download]', dialog);
  const deleteButton = $('[data-document-viewer-delete]', dialog);
  const closeButton = $('[data-document-viewer-close]', dialog);
  const loading = $('[data-document-viewer-loading]', dialog);
  const loadingText = $('[data-document-viewer-loading-text]', dialog);
  const errorBox = $('[data-document-viewer-error]', dialog);
  const frame = $('[data-document-viewer-frame]', dialog);
  const markdown = $('[data-document-viewer-markdown]', dialog);
  const source = $('[data-document-viewer-source]', dialog);
  const sourceCode = $('[data-document-viewer-source-code]', dialog);
  const editor = $('[data-document-viewer-editor]', dialog);
  const footnote = $('[data-document-viewer-footnote]', dialog);
  const bytesLabel = $('[data-document-viewer-bytes]', dialog);
  const csrfToken = $('meta[name="csrf-token"]')?.content || '';

  const state = {
    id: '', viewer: '', editable: false, content: '', version: '', starred: false,
    size: 0, editing: false, dirty: false, activeTab: 'preview', openSequence: 0
  };

  const formatBytes = (value) => {
    let n = Number(value || 0);
    if (n < 1024) return `${n} B`;
    const units = ['KiB', 'MiB', 'GiB', 'TiB'];
    let i = -1;
    do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1);
    return `${n.toFixed(1)} ${units[i]}`;
  };

  const toast = (message) => {
    const node = document.createElement('div');
    node.className = 'drive-unit-toast';
    node.textContent = message;
    document.body.append(node);
    window.setTimeout(() => node.remove(), 3200);
  };

  const fetchJSON = async (url, options = {}) => {
    const response = await fetch(url, { cache: 'no-store', ...options, headers: { Accept: 'application/json', ...(options.headers || {}) } });
    const contentType = response.headers.get('content-type') || '';
    const data = contentType.includes('application/json') ? await response.json() : { error: (await response.text()).trim() };
    if (!response.ok) {
      const err = new Error(data.error || 'La operación no pudo completarse.');
      err.status = response.status;
      err.data = data;
      throw err;
    }
    return data;
  };

  const showLoading = (message = 'Cargando…') => {
    if (loadingText) loadingText.textContent = message;
    if (loading) loading.hidden = false;
    if (errorBox) errorBox.hidden = true;
  };
  const hideLoading = () => { if (loading) loading.hidden = true; };
  const showError = (message) => {
    hideLoading();
    [frame, markdown, source, editor].forEach((node) => { if (node) node.hidden = true; });
    if (errorBox) { errorBox.textContent = message || 'No se pudo abrir el documento.'; errorBox.hidden = false; }
    if (status) status.textContent = 'No se pudo mostrar el archivo';
  };

  const updateExternalStar = (id, starred) => {
    if (!id) return;
    const escaped = CSS.escape(id);
    $$(`[data-selectable-file="${escaped}"], [data-download-file-id="${escaped}"]`).forEach((node) => {
      node.dataset.starred = String(Boolean(starred));
      node.classList.toggle('is-starred', Boolean(starred));
    });
  };

  const syncStarButton = (loadingState = false) => {
    if (!starButton) return;
    starButton.disabled = loadingState;
    starButton.classList.toggle('is-starred', state.starred);
    const label = loadingState ? 'Actualizando Destacados…' : (state.starred ? 'Quitar de Destacados' : 'Agregar a Destacados');
    starButton.setAttribute('aria-label', label);
    starButton.title = label;
  };

  const startSecureDownload = async () => {
    if (!state.id || !csrfToken) throw new Error('No se pudo preparar la descarga.');
    const data = await fetchJSON('/api/descargas', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8' },
      body: new URLSearchParams({ csrf_token: csrfToken, file_id: state.id })
    });
    if (!data.url) throw new Error('El servidor no devolvió una descarga válida.');
    const anchor = document.createElement('a');
    anchor.href = data.url;
    anchor.hidden = true;
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
  };

  const safeLink = (raw) => {
    const value = String(raw || '').trim();
    if (!value) return '';
    try {
      const parsed = new URL(value, window.location.origin);
      if (!['http:', 'https:', 'mailto:'].includes(parsed.protocol)) return '';
      return parsed.href;
    } catch (_) { return ''; }
  };

  const appendInlineMarkdown = (parent, text) => {
    let i = 0;
    const addText = (value) => { if (value) parent.append(document.createTextNode(value)); };
    while (i < text.length) {
      if (text.startsWith('**', i) || text.startsWith('__', i)) {
        const marker = text.slice(i, i + 2);
        const end = text.indexOf(marker, i + 2);
        if (end > i + 2) {
          const strong = document.createElement('strong');
          appendInlineMarkdown(strong, text.slice(i + 2, end));
          parent.append(strong); i = end + 2; continue;
        }
      }
      if (text.startsWith('~~', i)) {
        const end = text.indexOf('~~', i + 2);
        if (end > i + 2) {
          const del = document.createElement('del');
          appendInlineMarkdown(del, text.slice(i + 2, end));
          parent.append(del); i = end + 2; continue;
        }
      }
      if (text[i] === '`') {
        const end = text.indexOf('`', i + 1);
        if (end > i + 1) {
          const code = document.createElement('code');
          code.textContent = text.slice(i + 1, end);
          parent.append(code); i = end + 1; continue;
        }
      }
      if (text[i] === '*' || text[i] === '_') {
        const marker = text[i];
        const end = text.indexOf(marker, i + 1);
        if (end > i + 1) {
          const em = document.createElement('em');
          appendInlineMarkdown(em, text.slice(i + 1, end));
          parent.append(em); i = end + 1; continue;
        }
      }
      if (text[i] === '[') {
        const close = text.indexOf('](', i + 1);
        const end = close >= 0 ? text.indexOf(')', close + 2) : -1;
        if (close > i + 1 && end > close + 2) {
          const href = safeLink(text.slice(close + 2, end));
          if (href) {
            const link = document.createElement('a');
            link.href = href;
            link.target = '_blank';
            link.rel = 'noopener noreferrer';
            appendInlineMarkdown(link, text.slice(i + 1, close));
            parent.append(link); i = end + 1; continue;
          }
        }
      }
      let next = text.length;
      for (const token of ['**', '__', '~~', '`', '*', '_', '[']) {
        const pos = text.indexOf(token, i + 1);
        if (pos >= 0 && pos < next) next = pos;
      }
      addText(text.slice(i, next));
      i = next;
    }
  };

  const isMarkdownBlockStart = (line) => {
    const value = line || '';
    return /^\s*$/.test(value) || /^\s*```/.test(value) || /^\s{0,3}#{1,6}\s+/.test(value) || /^\s{0,3}>\s?/.test(value) || /^\s{0,3}([-*_])(?:\s*\1){2,}\s*$/.test(value) || /^\s*[-+*]\s+/.test(value) || /^\s*\d+[.)]\s+/.test(value);
  };

  const renderMarkdown = (container, text) => {
    container.replaceChildren();
    const lines = String(text || '').replace(/\r\n?/g, '\n').split('\n');
    let i = 0;
    while (i < lines.length) {
      const line = lines[i];
      if (/^\s*$/.test(line)) { i++; continue; }

      const fence = line.match(/^\s*```\s*([^\s`]*)\s*$/);
      if (fence) {
        const codeLines = [];
        i++;
        while (i < lines.length && !/^\s*```\s*$/.test(lines[i])) { codeLines.push(lines[i]); i++; }
        if (i < lines.length) i++;
        const pre = document.createElement('pre');
        const code = document.createElement('code');
        if (fence[1]) code.dataset.language = fence[1];
        code.textContent = codeLines.join('\n');
        pre.append(code); container.append(pre); continue;
      }

      const heading = line.match(/^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$/);
      if (heading) {
        const node = document.createElement(`h${heading[1].length}`);
        appendInlineMarkdown(node, heading[2]);
        container.append(node); i++; continue;
      }

      if (/^\s{0,3}([-*_])(?:\s*\1){2,}\s*$/.test(line)) {
        container.append(document.createElement('hr')); i++; continue;
      }

      if (/^\s{0,3}>\s?/.test(line)) {
        const quoteLines = [];
        while (i < lines.length && /^\s{0,3}>\s?/.test(lines[i])) {
          quoteLines.push(lines[i].replace(/^\s{0,3}>\s?/, '')); i++;
        }
        const quote = document.createElement('blockquote');
        appendInlineMarkdown(quote, quoteLines.join(' '));
        container.append(quote); continue;
      }

      const unordered = line.match(/^\s*[-+*]\s+(.+)$/);
      const ordered = line.match(/^\s*\d+[.)]\s+(.+)$/);
      if (unordered || ordered) {
        const list = document.createElement(unordered ? 'ul' : 'ol');
        const matcher = unordered ? /^\s*[-+*]\s+(.+)$/ : /^\s*\d+[.)]\s+(.+)$/;
        while (i < lines.length) {
          const match = lines[i].match(matcher);
          if (!match) break;
          const item = document.createElement('li');
          let itemText = match[1];
          const task = itemText.match(/^\[([ xX])\]\s+(.*)$/);
          if (task) {
            const checkbox = document.createElement('input');
            checkbox.type = 'checkbox'; checkbox.disabled = true; checkbox.checked = task[1].toLowerCase() === 'x';
            item.append(checkbox); itemText = task[2];
          }
          appendInlineMarkdown(item, itemText);
          list.append(item); i++;
        }
        container.append(list); continue;
      }

      const paragraphLines = [line.trim()];
      i++;
      while (i < lines.length && !isMarkdownBlockStart(lines[i])) {
        paragraphLines.push(lines[i].trim()); i++;
      }
      const paragraph = document.createElement('p');
      appendInlineMarkdown(paragraph, paragraphLines.join(' '));
      container.append(paragraph);
    }
  };

  const resetSurfaces = () => {
    if (errorBox) errorBox.hidden = true;
    if (frame) { frame.hidden = true; frame.removeAttribute('src'); frame.removeAttribute('srcdoc'); }
    if (markdown) { markdown.hidden = true; markdown.replaceChildren(); }
    if (source) source.hidden = true;
    if (sourceCode) sourceCode.textContent = '';
    if (editor) { editor.hidden = true; editor.value = ''; }
  };

  const buildIsolatedHTML = (raw) => {
    const parser = new DOMParser();
    const parsed = parser.parseFromString(String(raw || ''), 'text/html');
    parsed.querySelectorAll('script,iframe,frame,frameset,object,embed,applet,base,link').forEach((node) => node.remove());
    parsed.querySelectorAll('meta[http-equiv]').forEach((node) => node.remove());
    parsed.querySelectorAll('*').forEach((node) => {
      [...node.attributes].forEach((attribute) => {
        const name = attribute.name.toLowerCase();
        const value = String(attribute.value || '').trim();
        if (name.startsWith('on') || name === 'ping' || name === 'action' || name === 'formaction' || name === 'target') {
          node.removeAttribute(attribute.name);
          return;
        }
        if (name === 'href') {
          if (!value.startsWith('#')) node.removeAttribute(attribute.name);
          return;
        }
        if (name === 'src' || name === 'poster') {
          if (!/^(data:|blob:)/i.test(value)) node.removeAttribute(attribute.name);
          return;
        }
        if (name === 'srcset') node.removeAttribute(attribute.name);
      });
    });
    const head = parsed.head || parsed.documentElement.insertBefore(parsed.createElement('head'), parsed.body || null);
    const csp = parsed.createElement('meta');
    csp.setAttribute('http-equiv', 'Content-Security-Policy');
    csp.setAttribute('content', "default-src 'none'; script-src 'none'; connect-src 'none'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data:; object-src 'none'; frame-src 'none'; child-src 'none'; base-uri 'none'; form-action 'none'");
    head.prepend(csp);
    const isolationStyle = parsed.createElement('style');
    isolationStyle.textContent = 'html,body{min-height:100%;} body{margin:0;}';
    head.append(isolationStyle);
    return '<!doctype html>\n' + parsed.documentElement.outerHTML;
  };

  const typeLabel = (viewer) => ({ markdown: 'MD', html: 'HTML', text: 'TXT', pdf: 'PDF' }[viewer] || 'DOC');
  const statusLabel = (viewer) => ({ markdown: 'Vista Markdown local', html: 'Vista HTML segura · scripts y red desactivados', text: 'Archivo de texto', pdf: 'Documento PDF' }[viewer] || 'Documento');

  const setTab = (tab) => {
    if (state.editing) return;
    state.activeTab = tab === 'source' ? 'source' : 'preview';
    previewTab?.classList.toggle('is-active', state.activeTab === 'preview');
    sourceTab?.classList.toggle('is-active', state.activeTab === 'source');
    resetSurfaces();
    if (state.activeTab === 'source') {
      if (sourceCode) sourceCode.textContent = state.content;
      if (source) source.hidden = false;
      return;
    }
    if (state.viewer === 'markdown') {
      renderMarkdown(markdown, state.content);
      if (markdown) markdown.hidden = false;
    } else if (state.viewer === 'html') {
      if (frame) {
        // srcdoc + sandbox crea un documento con origen opaco. El HTML del
        // usuario no comparte DOM, CSS ni contexto de ejecución con Personal Cloud.
        frame.setAttribute('sandbox', '');
        frame.srcdoc = buildIsolatedHTML(state.content);
        frame.hidden = false;
      }
    } else if (state.viewer === 'text') {
      if (sourceCode) sourceCode.textContent = state.content;
      if (source) source.hidden = false;
    } else if (state.viewer === 'pdf') {
      if (frame) {
        frame.removeAttribute('sandbox');
        frame.src = `/archivo/${encodeURIComponent(state.id)}/pdf`;
        frame.hidden = false;
      }
    }
  };

  const leaveEditMode = () => {
    state.editing = false;
    state.dirty = false;
    if (editButton) editButton.hidden = !state.editable;
    if (saveButton) saveButton.hidden = true;
    if (cancelButton) cancelButton.hidden = true;
    if (tabs) tabs.hidden = !['markdown', 'html'].includes(state.viewer);
    if (status) status.textContent = statusLabel(state.viewer);
    setTab(state.activeTab);
  };

  const enterEditMode = () => {
    if (!state.editable || !editor) return;
    state.editing = true;
    state.dirty = false;
    resetSurfaces();
    editor.value = state.content;
    editor.hidden = false;
    if (tabs) tabs.hidden = true;
    if (editButton) editButton.hidden = true;
    if (saveButton) saveButton.hidden = false;
    if (cancelButton) cancelButton.hidden = false;
    if (status) status.textContent = 'Editando · Ctrl+S para guardar';
    window.setTimeout(() => editor.focus(), 0);
  };

  const loadTextContent = async (sequence) => {
    const data = await fetchJSON(`/api/archivo/${encodeURIComponent(state.id)}/contenido`);
    if (sequence !== state.openSequence) return false;
    state.content = data.content || '';
    state.version = data.version || '';
    state.size = Number(data.size || 0);
    state.editable = Boolean(data.editable);
    state.starred = Boolean(data.starred);
    if (bytesLabel) bytesLabel.textContent = formatBytes(state.size);
    syncStarButton(false);
    return true;
  };

  const open = async (fileID) => {
    const id = String(fileID || '').trim();
    if (!id) return false;
    const info = await fetchJSON(`/api/archivo/${encodeURIComponent(id)}/info`);
    const viewer = String(info.viewer || '');
    if (!['markdown', 'html', 'text', 'pdf'].includes(viewer)) return false;
    if (info.online === false) throw new Error('La unidad que contiene este archivo no está conectada.');

    state.openSequence++;
    const sequence = state.openSequence;
    state.id = id;
    state.viewer = viewer;
    state.editable = Boolean(info.editable);
    state.content = '';
    state.version = '';
    state.starred = Boolean(info.starred);
    state.size = Number(info.size || 0);
    state.editing = false;
    state.dirty = false;
    state.activeTab = 'preview';

    resetSurfaces();
    if (title) title.textContent = info.name || 'Documento';
    if (status) status.textContent = statusLabel(viewer);
    if (typeBadge) typeBadge.textContent = typeLabel(viewer);
    if (bytesLabel) bytesLabel.textContent = formatBytes(state.size);
    if (footnote) footnote.textContent = viewer === 'pdf' ? 'Esc para cerrar · usa Descargar para conservar una copia' : 'Esc para cerrar · Ctrl+S al editar';
    if (editButton) editButton.hidden = !state.editable;
    if (saveButton) saveButton.hidden = true;
    if (cancelButton) cancelButton.hidden = true;
    if (tabs) tabs.hidden = !['markdown', 'html'].includes(viewer);
    syncStarButton(false);
    if (!dialog.open) dialog.showModal();
    showLoading(viewer === 'pdf' ? 'Cargando PDF…' : 'Cargando documento…');

    try {
      if (viewer === 'pdf') {
        setTab('preview');
        // La carga real termina con el evento load del iframe.
      } else {
        if (!await loadTextContent(sequence)) return true;
        setTab('preview');
        // HTML se muestra dentro de un iframe aislado; mantenemos el loader
        // hasta que ese documento termine de cargar para evitar un flash blanco.
        if (viewer !== 'html') hideLoading();
      }
    } catch (error) {
      if (sequence === state.openSequence) showError(error.message || 'No se pudo abrir el documento.');
    }
    return true;
  };

  const close = () => {
    if (state.editing && state.dirty && !window.confirm('Hay cambios sin guardar. ¿Cerrar el archivo y descartarlos?')) return false;
    state.openSequence++;
    state.editing = false;
    state.dirty = false;
    resetSurfaces();
    if (loading) loading.hidden = true;
    dialog.close();
    return true;
  };

  frame?.addEventListener('load', () => {
    if (!dialog.open || !['pdf', 'html'].includes(state.viewer)) return;
    hideLoading();
  });
  previewTab?.addEventListener('click', () => setTab('preview'));
  sourceTab?.addEventListener('click', () => setTab('source'));
  editButton?.addEventListener('click', enterEditMode);
  editor?.addEventListener('input', () => { state.dirty = editor.value !== state.content; });
  editor?.addEventListener('keydown', (event) => {
    if (event.key === 'Tab') {
      event.preventDefault();
      const start = editor.selectionStart, end = editor.selectionEnd;
      editor.setRangeText('  ', start, end, 'end');
      editor.dispatchEvent(new Event('input'));
    }
  });
  cancelButton?.addEventListener('click', () => {
    if (state.dirty && !window.confirm('¿Descartar los cambios realizados?')) return;
    leaveEditMode();
  });
  saveButton?.addEventListener('click', async () => {
    if (!state.editing || !state.id || !editor) return;
    saveButton.disabled = true;
    if (status) status.textContent = 'Guardando…';
    try {
      const result = await fetchJSON(`/api/archivo/${encodeURIComponent(state.id)}/contenido`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json; charset=utf-8', 'X-CSRF-Token': csrfToken },
        body: JSON.stringify({ content: editor.value, version: state.version })
      });
      state.content = editor.value;
      state.version = result.version || state.version;
      state.size = Number(result.size ?? new TextEncoder().encode(state.content).length);
      if (bytesLabel) bytesLabel.textContent = formatBytes(state.size);
      leaveEditMode();
      toast('Cambios guardados');
    } catch (error) {
      if (status) status.textContent = error.status === 409 ? 'Conflicto: el archivo cambió fuera del editor' : 'No se pudo guardar';
      window.alert(error.message || 'No se pudo guardar el archivo.');
    } finally { saveButton.disabled = false; }
  });
  downloadButton?.addEventListener('click', async () => {
    downloadButton.disabled = true;
    try { await startSecureDownload(); }
    catch (error) { window.alert(error.message || 'No se pudo descargar el archivo.'); }
    finally { downloadButton.disabled = false; }
  });
  starButton?.addEventListener('click', async () => {
    if (!state.id) return;
    syncStarButton(true);
    try {
      const result = await fetchJSON(`/api/archivo/${encodeURIComponent(state.id)}/destacar`, {
        method: 'POST',
        headers: { 'X-CSRF-Token': csrfToken, 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8' },
        body: new URLSearchParams({ starred: String(!state.starred) })
      });
      state.starred = Boolean(result.starred);
      updateExternalStar(state.id, state.starred);
      toast(state.starred ? 'Agregado a Destacados' : 'Quitado de Destacados');
    } catch (error) { window.alert(error.message || 'No se pudo actualizar Destacados.'); }
    finally { syncStarButton(false); }
  });
  deleteButton?.addEventListener('click', async () => {
    if (!state.id) return;
    try {
      const actions = window.PersonalCloudActions;
      if (!actions?.deleteFiles) throw new Error('La acción de eliminación todavía no está disponible.');
      const deleted = await actions.deleteFiles([state.id], { reload: false, source: 'viewer' });
      if (deleted) {
        state.openSequence++;
        state.editing = false;
        state.dirty = false;
        resetSurfaces();
        dialog.close();
        window.setTimeout(() => window.location.reload(), 180);
      }
    } catch (error) { toast(error.message || 'No se pudo eliminar el archivo.'); }
  });
  dialog.addEventListener('contextmenu', (event) => {
    if (!state.id || event.target.closest('textarea,input,select')) return;
    const actions = window.PersonalCloudActions;
    if (!actions?.showFileContext) return;
    event.preventDefault();
    actions.showFileContext(event.clientX, event.clientY, state.id, false);
  });
  closeButton?.addEventListener('click', close);
  dialog.addEventListener('cancel', (event) => { event.preventDefault(); close(); });
  dialog.addEventListener('click', (event) => { if (event.target === dialog) close(); });
  window.addEventListener('keydown', (event) => {
    if (!dialog.open || !state.editing || !(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 's') return;
    event.preventDefault();
    saveButton?.click();
  }, true);

  // Abrir con el visor desde Mi unidad, Recientes, Destacados y Página principal.
  document.addEventListener('click', (event) => {
    if (event.defaultPrevented || event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    if (document.body.classList.contains('selection-mode') || event.target.closest('button')) return;
    const carrier = event.target.closest('[data-download-file-id][data-viewer]');
    const viewer = carrier?.dataset.viewer || '';
    const id = carrier?.dataset.downloadFileId || '';
    if (!id || !['markdown', 'html', 'text', 'pdf'].includes(viewer)) return;
    event.preventDefault();
    if (carrier.dataset.offline === 'true') {
      toast('La unidad de este archivo no está conectada');
      return;
    }
    open(id).catch((error) => toast(error.message || 'No se pudo abrir el archivo'));
  });

  window.PersonalCloudDocumentViewer = { open };
})();
