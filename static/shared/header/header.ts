function registerHeaderListeners() {
  const header = document.querySelector('.js-header');
  const menuButtons = document.querySelectorAll('.js-headerMenuButton');
  menuButtons.forEach(button => {
    button.addEventListener('click', e => {
      e.preventDefault();
      header?.classList.toggle('is-active');
      button.setAttribute('aria-expanded', String(header?.classList.contains('is-active')));
    });
  });

  const scrim = document.querySelector('.js-scrim');
  scrim?.addEventListener('click', e => {
    e.preventDefault();
    header?.classList.remove('is-active');
    menuButtons.forEach(button => {
      button.setAttribute('aria-expanded', String(header?.classList.contains('is-active')));
    });
  });
}

function registerSearchFormListeners() {
  const searchForm = document.querySelector('.js-searchForm');
  const expandSearch = document.querySelector('.js-expandSearch');
  const input = searchForm?.querySelector('input');
  const headerLogo = document.querySelector('.js-headerLogo');
  const menuButton = document.querySelector('.js-headerMenuButton');
  expandSearch?.addEventListener('click', () => {
    searchForm?.classList.add('go-SearchForm--expanded');
    headerLogo?.classList.add('go-Header-logo--hidden');
    menuButton?.classList.add('go-Header-navOpen--hidden');
    input?.focus();
  });
  document?.addEventListener('click', e => {
    if (!searchForm?.contains(e.target as Node)) {
      searchForm?.classList.remove('go-SearchForm--expanded');
      headerLogo?.classList.remove('go-Header-logo--hidden');
      menuButton?.classList.remove('go-Header-navOpen--hidden');
    }
  });
}

/**
 * Listen for changes in the search dropdown.
 *
 * TODO(https://golang.org/issue/44142): Fix this flow:
 * A user will likely expect to submit the search again after selecting the
 * type of search. The change event should trigger a form submission, so that the
 * search event is still captured in analytics without a manual instrumentation.
 */
document.querySelectorAll('.js-searchModeSelect').forEach(el => {
  el.addEventListener('change', e => {
    const urlSearchParams = new URLSearchParams(window.location.search);
    const params = Object.fromEntries(urlSearchParams.entries());
    const query = params['q'];
    if (query) {
      window.location.search = `q=${query}&m=${(e.target as HTMLSelectElement).value}`;
    }
  });
});

registerHeaderListeners();
registerSearchFormListeners();
