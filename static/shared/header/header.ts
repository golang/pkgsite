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
  const BREAKPOINT = 512;
  const logo = document.querySelector('.js-headerLogo');
  const form = document.querySelector<HTMLFormElement>('.js-searchForm');
  const button = document.querySelector('.js-searchFormSubmit');
  const input = form?.querySelector('input');

  renderForm();

  window.addEventListener('resize', renderForm);

  function renderForm() {
    if (window.innerWidth > BREAKPOINT) {
      logo?.classList.remove('go-Header-logo--hidden');
      form?.classList.remove('go-SearchForm--open');
      input?.removeEventListener('focus', showSearchBox);
      input?.removeEventListener('keypress', handleKeypress);
      input?.removeEventListener('focusout', hideSearchBox);
    } else {
      button?.addEventListener('click', handleSearchClick);
      input?.addEventListener('focus', showSearchBox);
      input?.addEventListener('keypress', handleKeypress);
      input?.addEventListener('focusout', hideSearchBox);
    }
  }

  /**
   * Submits form if Enter key is pressed
   */
  function handleKeypress(e: KeyboardEvent) {
    if (e.key === 'Enter') form?.submit();
  }

  /**
   * Shows the search box when it receives focus (expands it from
   * just the spyglass if we're on mobile).
   */
  function showSearchBox() {
    logo?.classList.add('go-Header-logo--hidden');
    form?.classList.add('go-SearchForm--open');
  }

  /**
   * Hides the search box (shrinks to just the spyglass icon).
   */
  function hideSearchBox() {
    logo?.classList.remove('go-Header-logo--hidden');
    form?.classList.remove('go-SearchForm--open');
  }

  /**
   * Expands the searchbox so input is visible and gives
   * the input focus.
   */
  function handleSearchClick(e: Event) {
    e.preventDefault();

    showSearchBox();
    input?.focus();
  }
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
