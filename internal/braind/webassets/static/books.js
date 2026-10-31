(() => {
  const toolbar = document.querySelector('.books-toolbar');
  const search = document.querySelector('#book-search');
  const rows = [...document.querySelectorAll('.book-row')];
  const filters = [...document.querySelectorAll('.books-filter')];
  const count = document.querySelector('.books-count');
  const noResults = document.querySelector('.books-no-results');
  if (!toolbar || !search || !rows.length || !count || !noResults) return;

  let statusFilter = 'all';

  function updateShelf() {
    const query = search.value.trim().toLocaleLowerCase();
    let visible = 0;

    for (const row of rows) {
      const matchesStatus = statusFilter === 'all' || row.dataset.status.trim().toLocaleLowerCase() === statusFilter;
      const details = row.querySelector('.book-main');
      const rating = details.querySelector('.book-rating')?.getAttribute('aria-label') || '';
      const searchableText = `${details.textContent} ${rating}`.toLocaleLowerCase();
      const matchesSearch = !query || searchableText.includes(query);
      row.hidden = !(matchesStatus && matchesSearch);
      if (!row.hidden) visible++;
    }

    count.textContent = `${visible} ${visible === 1 ? 'book' : 'books'}`;
    noResults.hidden = visible !== 0;
  }

  search.addEventListener('input', updateShelf);
  for (const button of filters) {
    button.addEventListener('click', () => {
      statusFilter = button.dataset.filter;
      for (const item of filters) {
        const selected = item === button;
        item.classList.toggle('active', selected);
        item.setAttribute('aria-pressed', String(selected));
      }
      updateShelf();
    });
  }

  toolbar.hidden = false;
})();
