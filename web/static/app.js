(() => {
  const activeIndex = document.querySelector('[data-index-active="true"]');
  if (activeIndex) {
    window.setTimeout(() => window.location.reload(), 2500);
  }
})();
