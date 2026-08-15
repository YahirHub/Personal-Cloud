(() => {
  const $ = (s, root = document) => root.querySelector(s);
  const $$ = (s, root = document) => [...root.querySelectorAll(s)];

  const formatBytes = (n) => {
    n = Number(n || 0); if (n < 1024) return `${n} B`;
    const units = ['KiB','MiB','GiB','TiB','PiB']; let v=n, i=-1;
    do { v/=1024; i++; } while (v>=1024 && i<units.length-1);
    return `${v.toFixed(1)} ${units[i]}`;
  };
  const formatTime = (v) => { const d=new Date(v); return Number.isNaN(d.getTime())?'—':d.toLocaleString(); };

  // Progreso de indexación en vivo.
  const indexCards = $$('[data-index-card]');
  if (indexCards.length) {
    const update = async () => {
      try {
        const res = await fetch('/api/indexacion', {headers:{Accept:'application/json'}, cache:'no-store'});
        if (!res.ok) return;
        const jobs = await res.json(); const byId = new Map(jobs.map(j => [j.storage_id, j]));
        indexCards.forEach(card => {
          const job=byId.get(card.dataset.indexCard); if (!job) return;
          const box=$('[data-index-progress]',card), bar=$('[data-index-bar]',card), label=$('[data-index-label]',card), pct=$('[data-index-percent]',card), detail=$('[data-index-detail]',card), btn=$('[data-index-button]',card);
          const active=['queued','counting','scanning'].includes(job.state); box?.classList.toggle('active',active); if(btn) btn.disabled=active;
          if(bar) bar.value=job.percent||0; if(pct) pct.textContent=`${job.percent||0}%`;
          if(label){ if(job.state==='queued')label.textContent='En cola…'; else if(job.state==='counting')label.textContent='Contando archivos…'; else if(job.state==='scanning')label.textContent=`Indexando ${job.scanned} de ${job.total}`; else if(job.state==='done')label.textContent=`Indexación completa: ${job.scanned} archivos`; else if(job.state==='error')label.textContent=`Error: ${job.error}`; }
          if(detail){ if(job.state==='scanning')detail.textContent=`${job.images||0} imágenes · ${job.videos||0} videos · ${job.audio||0} audios`; else if(job.state==='done')detail.textContent='Última indexación terminada'; else detail.textContent=''; }
        });
      } catch (_) {}
    };
    update(); window.setInterval(update, 1000);
  }

  // Widget contextual de subida.
  const uploadDialog=$('[data-upload-dialog]');
  $('[data-open-upload]')?.addEventListener('click',()=>uploadDialog?.showModal());
  $('[data-close-upload]')?.addEventListener('click',()=>uploadDialog?.close());
  uploadDialog?.addEventListener('click',e=>{ if(e.target===uploadDialog) uploadDialog.close(); });

  // Galería y visor multimedia offline.
  const grid=$('[data-gallery-grid]'), sentinel=$('[data-gallery-sentinel]');
  const viewer=$('[data-media-viewer]'), stage=$('[data-viewer-stage]'); let currentIndex=-1, zoom=1, loadingGallery=false;
  const mediaCards=()=>$$('[data-media-id]',grid||document);
  const makeMediaCard=(item)=>{
    const b=document.createElement('button'); b.type='button'; b.className='photo-card media-card';
    Object.assign(b.dataset,{mediaId:item.id,kind:item.kind,name:item.name,original:item.original_url,preview:item.preview_url||'',thumbnail:item.thumbnail_url||''}); b.title=`Abrir ${item.name}`;
    if(item.thumbnail_url){const img=document.createElement('img');img.src=item.thumbnail_url;img.loading='lazy';img.alt='';b.append(img);}else{const ph=document.createElement('div');ph.className='photo-placeholder';ph.textContent=item.kind==='video'?'▶':item.kind==='audio'?'♪':'▧';b.append(ph);}
    if(item.kind==='video'||item.kind==='audio'){const badge=document.createElement('span');badge.className='media-badge';badge.textContent=item.kind==='video'?'Video':'Audio';b.append(badge);}
    const meta=document.createElement('div');meta.className='photo-meta';const strong=document.createElement('strong');strong.textContent=item.name;const small=document.createElement('small');small.textContent=`${formatTime(item.mod_time)} · ${formatBytes(item.size)}`;meta.append(strong,small);b.append(meta);return b;
  };
  const loadMoreGallery=async()=>{
    if(!grid||loadingGallery||grid.dataset.hasMore!=='true')return; loadingGallery=true;
    try{const offset=Number(grid.dataset.next||0);const res=await fetch(`/api/galeria?offset=${offset}&limit=80`,{headers:{Accept:'application/json'},cache:'no-store'});if(!res.ok)return;const data=await res.json();data.items.forEach(i=>grid.append(makeMediaCard(i)));grid.dataset.next=data.next;grid.dataset.hasMore=String(data.has_more);if(sentinel&&!data.has_more)sentinel.textContent='Fin de la galería';}finally{loadingGallery=false;}
  };
  if(grid?.dataset.mode==='infinito'&&sentinel){new IntersectionObserver(entries=>{if(entries.some(e=>e.isIntersecting))loadMoreGallery();},{rootMargin:'500px'}).observe(sentinel);}

  const setZoom=(value)=>{zoom=Math.max(.5,Math.min(5,value));const visual=$('[data-viewer-visual]',stage);if(visual)visual.style.transform=`scale(${zoom})`;};
  const showMedia=(index)=>{
    const cards=mediaCards(); if(!cards.length)return; currentIndex=(index+cards.length)%cards.length;const c=cards[currentIndex];zoom=1;stage.replaceChildren();
    $('[data-viewer-title]',viewer).textContent=c.dataset.name||'';const original=$('[data-viewer-original]',viewer);original.href=c.dataset.original;
    if(c.dataset.kind==='image'){
      const img=document.createElement('img');img.className='viewer-image';img.dataset.viewerVisual='1';img.alt=c.dataset.name||'';img.src=c.dataset.preview||c.dataset.thumbnail||c.dataset.original;stage.append(img);
      if(c.dataset.original&&c.dataset.original!==img.src){const full=new Image();full.onload=()=>{if(currentIndex>=0&&stage.contains(img))img.src=c.dataset.original;};full.src=c.dataset.original;}
    }else if(c.dataset.kind==='video'){
      const video=document.createElement('video');video.className='viewer-video';video.dataset.viewerVisual='1';video.controls=true;video.autoplay=true;video.playsInline=true;video.src=c.dataset.original;if(c.dataset.thumbnail)video.poster=c.dataset.thumbnail;stage.append(video);
    }else{
      const wrap=document.createElement('div');wrap.className='viewer-audio';if(c.dataset.thumbnail){const img=document.createElement('img');img.src=c.dataset.thumbnail;img.alt='';wrap.append(img);}const audio=document.createElement('audio');audio.controls=true;audio.autoplay=true;audio.src=c.dataset.original;wrap.append(audio);stage.append(wrap);
    }
    if(!viewer.open)viewer.showModal();
  };
  grid?.addEventListener('click',e=>{const card=e.target.closest('[data-media-id]');if(card)showMedia(mediaCards().indexOf(card));});
  const stepMedia=async(delta)=>{
    const before=mediaCards().length;
    if(delta>0&&currentIndex===before-1&&grid?.dataset.hasMore==='true')await loadMoreGallery();
    showMedia(currentIndex+delta);
  };
  $('[data-viewer-close]',viewer)?.addEventListener('click',()=>viewer.close()); $('[data-viewer-prev]',viewer)?.addEventListener('click',()=>stepMedia(-1)); $('[data-viewer-next]',viewer)?.addEventListener('click',()=>stepMedia(1));
  viewer?.addEventListener('close',()=>{stage?.querySelectorAll('video,audio').forEach(m=>{m.pause();m.removeAttribute('src');m.load();});currentIndex=-1;zoom=1;});
  viewer?.addEventListener('click',e=>{if(e.target===viewer)viewer.close();});
  document.addEventListener('keydown',e=>{
    if(!viewer?.open)return;const tag=document.activeElement?.tagName;if(tag==='INPUT'||tag==='TEXTAREA'||tag==='SELECT')return;
    const k=e.key.toLowerCase();if(k==='arrowleft'||k==='a'){e.preventDefault();stepMedia(-1);}else if(k==='arrowright'||k==='d'){e.preventDefault();stepMedia(1);}else if(k==='w'){e.preventDefault();setZoom(zoom+.15);}else if(k==='s'){e.preventDefault();setZoom(zoom-.15);}else if(k==='escape'){viewer.close();}
  });

  // Listado continuo reutilizable de Archivos.
  const fileList=$('[data-files-list]'), fileSentinel=$('[data-files-sentinel]');let loadingFiles=false;
  const makeFileRow=(item)=>{const a=document.createElement('a');a.className='file-row';a.setAttribute('role','listitem');a.href=item.is_dir?item.url:item.download_url;const kind=document.createElement('span');kind.className='file-kind'+(item.is_dir?' folder':'');kind.textContent=item.is_dir?'▣':item.kind==='image'?'▧':item.kind==='video'?'▶':item.kind==='audio'?'♪':item.kind==='document'?'▤':item.kind==='archive'?'▥':'•';const main=document.createElement('span');main.className='file-main';const strong=document.createElement('strong');strong.textContent=item.name;const small=document.createElement('small');small.textContent=item.is_dir?`Carpeta${item.offline?' · unidad desconectada':''}`:`${item.kind} · ${formatTime(item.mod_time)}${item.offline?' · original no disponible hasta reconectar':''}`;main.append(strong,small);const size=document.createElement('span');size.className='file-size';size.textContent=item.is_dir?'—':formatBytes(item.size);a.append(kind,main,size);return a;};
  const loadMoreFiles=async()=>{if(!fileList||loadingFiles||fileList.dataset.hasMore!=='true')return;loadingFiles=true;try{const q=new URLSearchParams({path:fileList.dataset.path,offset:fileList.dataset.next||'0',limit:'100'});const res=await fetch(`/api/archivos/listado?${q}`,{headers:{Accept:'application/json'},cache:'no-store'});if(!res.ok)return;const data=await res.json();data.items.forEach(i=>fileList.append(makeFileRow(i)));fileList.dataset.next=data.next;fileList.dataset.hasMore=String(data.has_more);if(fileSentinel&&!data.has_more)fileSentinel.textContent='Fin de la carpeta';}finally{loadingFiles=false;}};
  if(fileList?.dataset.mode==='infinito'&&fileSentinel){new IntersectionObserver(entries=>{if(entries.some(e=>e.isIntersecting))loadMoreFiles();},{rootMargin:'500px'}).observe(fileSentinel);}
})();
