(() => {
  const player = document.querySelector('[data-public-video-player]');
  if (!player) return;
  let video = player.querySelector('[data-public-video]');
  if (!video) return;

  const play = player.querySelector('[data-public-video-play]');
  const playIcon = player.querySelector('[data-public-play-icon]');
  const pauseIcon = player.querySelector('[data-public-pause-icon]');
  const progress = player.querySelector('[data-public-video-progress]');
  const current = player.querySelector('[data-public-video-current]');
  const duration = player.querySelector('[data-public-video-duration]');
  const mute = player.querySelector('[data-public-video-mute]');
  const volume = player.querySelector('[data-public-video-volume]');
  const speed = player.querySelector('[data-public-video-speed]');
  const fullscreen = player.querySelector('[data-public-video-fullscreen]');
  const qualityControl = player.querySelector('[data-public-video-quality-control]');
  const qualitySelect = player.querySelector('[data-public-video-quality]');
  const qualityStatus = player.querySelector('[data-public-video-quality-status]');
  const originalURL = player.dataset.originalUrl || video.currentSrc || video.src;
  const qualitiesURL = player.dataset.qualitiesUrl || '';
  const prepareURL = player.dataset.prepareUrl || '';
  const statusURL = player.dataset.statusUrl || '';
  const connection = navigator.connection || navigator.mozConnection || navigator.webkitConnection;
  let profiles = [];
  let currentQuality = 'original';
  let pendingQuality = '';
  let autoQuality = true;
  let measuredMbps = 0;
  let measuredAt = 0;
  let bandwidthFactor = 1;
  let switchSequence = 0;
  let autoTimer = 0;
  let progressFrame = 0;

  const fmt = (seconds) => {
    if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
    const s = Math.floor(seconds % 60).toString().padStart(2, '0');
    const m = Math.floor(seconds / 60) % 60;
    const h = Math.floor(seconds / 3600);
    return h ? `${h}:${m.toString().padStart(2, '0')}:${s}` : `${m}:${s}`;
  };
  const withParam = (raw, key, value) => {
    const url = new URL(raw, window.location.origin);
    url.searchParams.set(key, value);
    return `${url.pathname}${url.search}`;
  };
  const sync = () => {
    if (!video) return;
    if (playIcon) playIcon.hidden = !video.paused;
    if (pauseIcon) pauseIcon.hidden = video.paused;
    if (current) current.textContent = fmt(video.currentTime);
    if (duration) duration.textContent = fmt(video.duration);
    if (progress && Number.isFinite(video.duration) && video.duration > 0) progress.value = (video.currentTime / video.duration) * 1000;
    if (volume) volume.value = String(video.muted ? 0 : video.volume);
  };
  const stopProgress = () => { if (progressFrame) cancelAnimationFrame(progressFrame); progressFrame = 0; };
  const progressLoop = () => {
    progressFrame = 0;
    if (!video || video.paused || video.ended) return;
    sync();
    progressFrame = requestAnimationFrame(progressLoop);
  };
  const startProgress = () => { stopProgress(); progressFrame = requestAnimationFrame(progressLoop); };

  const browserBandwidth = () => {
    const downlink = Number(connection?.downlink);
    if (Number.isFinite(downlink) && downlink > 0) return downlink;
    switch (connection?.effectiveType) {
      case 'slow-2g': return .12;
      case '2g': return .35;
      case '3g': return 1.4;
      case '4g': return 8;
      default: return 0;
    }
  };
  const effectiveBandwidth = () => {
    const fresh = measuredMbps > 0 && Date.now() - measuredAt < 90_000;
    const base = fresh ? measuredMbps : browserBandwidth();
    return base > 0 ? base * bandwidthFactor : 0;
  };
  const qualityBudget = { 360: .9, 480: 1.6, 720: 3.2, 1080: 5.8 };
  const numericProfiles = () => profiles.filter((p) => /^\d+$/.test(p.id || '')).sort((a, b) => Number(a.id) - Number(b.id));
  const chooseAutoQuality = () => {
    const options = numericProfiles();
    if (!options.length) return 'original';
    const displayHeight = Math.max(240, player.clientHeight || window.innerHeight || 720) * Math.min(1.5, Math.max(1, window.devicePixelRatio || 1)) * 1.15;
    let candidates = options.filter((p) => Number(p.id) <= displayHeight);
    if (!candidates.length) candidates = [options[0]];
    const mbps = effectiveBandwidth();
    if (mbps > 0) {
      const budget = mbps * .68;
      const fits = candidates.filter((p) => (qualityBudget[Number(p.id)] || Infinity) <= budget);
      return (fits.length ? fits[fits.length - 1] : options[0]).id;
    }
    const fallback = candidates.filter((p) => Number(p.id) <= 720);
    return (fallback.length ? fallback[fallback.length - 1] : candidates[candidates.length - 1]).id;
  };
  const describeAuto = (quality) => {
    const mbps = effectiveBandwidth();
    if (quality === 'original') return 'Auto · Original';
    return mbps > 0 ? `Auto · ${quality}p · ~${mbps.toFixed(1)} Mbps` : `Auto · ${quality}p`;
  };
  const measureBandwidth = async () => {
    if (!originalURL) return;
    try {
      const started = performance.now();
      const controller = new AbortController();
      const response = await fetch(originalURL, { headers: { Range: 'bytes=0-524287' }, cache: 'no-store', signal: controller.signal });
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
        measuredMbps = Math.min(1000, (received * 8) / seconds / 1_000_000);
        measuredAt = Date.now();
      }
    } catch (_) {}
  };

  const waitMedia = (node, events, timeout = 20_000) => new Promise((resolve, reject) => {
    const names = Array.isArray(events) ? events : [events];
    let done = false;
    const cleanup = () => { names.forEach((name) => node.removeEventListener(name, ready)); node.removeEventListener('error', failed); clearTimeout(timer); };
    const ready = () => { if (done) return; done = true; cleanup(); resolve(); };
    const failed = () => { if (done) return; done = true; cleanup(); reject(new Error('No se pudo cargar la resolución seleccionada.')); };
    const timer = setTimeout(failed, timeout);
    names.forEach((name) => node.addEventListener(name, ready, { once: true }));
    node.addEventListener('error', failed, { once: true });
  });

  const configureVideo = (node) => {
    video = node;
    if (volume) node.volume = Math.max(0, Math.min(1, Number(volume.value || 1)));
    if (speed) node.playbackRate = Number(speed.value) || 1;
    ['loadedmetadata', 'durationchange', 'seeking', 'seeked', 'ended', 'volumechange', 'ratechange'].forEach((name) => node.addEventListener(name, sync));
    node.addEventListener('play', () => { sync(); startProgress(); });
    node.addEventListener('pause', () => { stopProgress(); sync(); });
    node.addEventListener('waiting', () => { if (autoQuality) { bandwidthFactor = Math.max(.42, bandwidthFactor * .72); scheduleAuto(); } });
    node.addEventListener('stalled', () => { if (autoQuality) { bandwidthFactor = Math.max(.42, bandwidthFactor * .8); scheduleAuto(); } });
    node.addEventListener('click', () => node.paused ? node.play().catch(() => {}) : node.pause());
    node.addEventListener('dblclick', () => player.requestFullscreen?.());
    sync();
  };

  const waitVariant = async (quality, requestID) => {
    while (requestID === switchSequence) {
      await new Promise((resolve) => setTimeout(resolve, 850));
      const response = await fetch(withParam(statusURL, 'quality', quality), { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) throw new Error((await response.text()).trim() || 'No se pudo consultar la conversión.');
      const state = await response.json();
      if (state.state === 'ready') return state;
      if (state.state === 'error') throw new Error(state.error || 'No se pudo generar esta resolución.');
      if (qualityStatus && requestID === switchSequence) qualityStatus.textContent = `${quality}p · preparando…`;
    }
    throw new Error('cancelado');
  };

  const swapSource = async (url, requestID) => {
    if (!video || requestID !== switchSequence) throw new Error('cancelado');
    const old = video;
    const shouldResume = !old.paused && !old.ended;
    const preferences = { muted: old.muted, volume: old.volume, speed: old.playbackRate };
    const staged = document.createElement('video');
    staged.className = 'public-video-staging';
    staged.preload = 'auto'; staged.playsInline = true; staged.muted = true; staged.volume = 0; staged.playbackRate = preferences.speed; staged.src = url;
    player.insertBefore(staged, player.querySelector('.viewer-video-controls'));
    try {
      if (staged.readyState < HTMLMediaElement.HAVE_METADATA) await waitMedia(staged, 'loadedmetadata');
      if (requestID !== switchSequence || video !== old) throw new Error('cancelado');
      const target = Number.isFinite(old.currentTime) ? old.currentTime : 0;
      if (target > 0 && Number.isFinite(staged.duration)) {
        staged.currentTime = Math.min(target, Math.max(0, staged.duration - .05));
        await waitMedia(staged, 'seeked');
      }
      if (staged.readyState < HTMLMediaElement.HAVE_FUTURE_DATA) await waitMedia(staged, 'canplay');
      if (shouldResume) await staged.play();
      if (requestID !== switchSequence || video !== old) throw new Error('cancelado');
      old.pause(); old.remove();
      staged.classList.remove('public-video-staging');
      staged.dataset.publicVideo = '';
      staged.muted = preferences.muted; staged.volume = preferences.volume; staged.playbackRate = preferences.speed;
      configureVideo(staged);
      if (!shouldResume) staged.pause();
    } catch (error) {
      staged.pause(); staged.removeAttribute('src'); staged.load(); staged.remove();
      throw error;
    }
  };

  const changeQuality = async (quality, mode = 'manual') => {
    if (!quality || pendingQuality || quality === currentQuality) {
      if (qualityStatus && quality === currentQuality) qualityStatus.textContent = mode === 'auto' ? describeAuto(quality) : (quality === 'original' ? 'Original' : `${quality}p`);
      return;
    }
    const requestID = ++switchSequence;
    pendingQuality = quality;
    if (qualityStatus) qualityStatus.textContent = mode === 'auto' ? `Auto · preparando ${quality === 'original' ? 'Original' : `${quality}p`}…` : `${quality === 'original' ? 'Original' : `${quality}p`} · preparando…`;
    try {
      let url = originalURL;
      if (quality !== 'original') {
        const response = await fetch(prepareURL, {
          method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded;charset=UTF-8', Accept: 'application/json', 'X-Personal-Cloud-Share': 'video' },
          body: new URLSearchParams({ quality }), cache: 'no-store'
        });
        if (!response.ok && response.status !== 202) throw new Error((await response.text()).trim() || 'No se pudo preparar el video.');
        let state = await response.json();
        if (state.state !== 'ready') state = await waitVariant(quality, requestID);
        url = state.url;
      }
      if (requestID !== switchSequence) return;
      await swapSource(url, requestID);
      currentQuality = quality;
      if (qualityStatus) qualityStatus.textContent = mode === 'auto' ? describeAuto(quality) : (quality === 'original' ? 'Original' : `${quality}p`);
    } catch (error) {
      if (error.message !== 'cancelado' && requestID === switchSequence && qualityStatus) qualityStatus.textContent = error.message || 'No se pudo cambiar la calidad.';
    } finally {
      if (requestID === switchSequence) pendingQuality = '';
    }
  };

  const scheduleAuto = () => {
    clearTimeout(autoTimer);
    autoTimer = setTimeout(() => {
      if (!autoQuality || pendingQuality) return;
      const target = chooseAutoQuality();
      if (qualitySelect) qualitySelect.value = 'auto';
      if (target !== currentQuality) changeQuality(target, 'auto');
      else if (qualityStatus) qualityStatus.textContent = describeAuto(target);
    }, 220);
  };

  const setupQualities = async () => {
    if (!qualitiesURL || !qualityControl || !qualitySelect) return;
    try {
      const response = await fetch(qualitiesURL, { headers: { Accept: 'application/json' }, cache: 'no-store' });
      if (!response.ok) return;
      const data = await response.json();
      if (!data.ffmpeg || !Array.isArray(data.profiles) || data.profiles.length <= 1) return;
      profiles = data.profiles;
      qualitySelect.replaceChildren(new Option('Auto', 'auto'));
      profiles.forEach((profile) => {
        const option = new Option(profile.label, profile.id);
        if (profile.state === 'ready' && profile.id !== 'original') option.textContent += ' · lista';
        qualitySelect.append(option);
      });
      qualitySelect.value = 'auto';
      qualityControl.hidden = false;
      if (qualityStatus) qualityStatus.textContent = 'Auto · midiendo ancho de banda…';
      qualitySelect.addEventListener('change', () => {
        const selected = qualitySelect.value;
        qualitySelect.blur();
        if (selected === 'auto') {
          autoQuality = true; bandwidthFactor = 1; switchSequence += 1; pendingQuality = ''; scheduleAuto();
        } else {
          autoQuality = false; changeQuality(selected, 'manual');
        }
      });
      await measureBandwidth();
      scheduleAuto();
    } catch (_) {}
  };

  play?.addEventListener('click', () => video?.paused ? video.play().catch(() => {}) : video?.pause());
  progress?.addEventListener('input', () => { if (video && Number.isFinite(video.duration) && video.duration > 0) video.currentTime = (Number(progress.value) / 1000) * video.duration; });
  volume?.addEventListener('input', () => { if (!video) return; video.volume = Math.max(0, Math.min(1, Number(volume.value))); video.muted = video.volume === 0; sync(); });
  mute?.addEventListener('click', () => { if (!video) return; video.muted = !video.muted; if (!video.muted && video.volume === 0) video.volume = 1; sync(); });
  speed?.addEventListener('change', () => { if (!video) return; video.playbackRate = Number(speed.value) || 1; speed.blur(); });
  fullscreen?.addEventListener('click', () => player.requestFullscreen?.());
  connection?.addEventListener?.('change', () => { measuredAt = 0; bandwidthFactor = 1; scheduleAuto(); });
  window.addEventListener('resize', scheduleAuto);
  document.addEventListener('fullscreenchange', scheduleAuto);

  configureVideo(video);
  setupQualities();
})();
