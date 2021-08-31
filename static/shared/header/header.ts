/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

export function registerHeaderListeners(): void {
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

export function registerSearchFormListeners(): void {
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
