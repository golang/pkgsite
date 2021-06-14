/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

// Keyboard shortcuts:
// - Pressing '/' focuses the search box
// - Pressing 'y' changes the browser URL to the canonical URL
// without triggering a reload.

const searchInput = document.querySelector<HTMLInputElement>('.js-searchFocus');
const canonicalURLPath = document.querySelector<HTMLDivElement>('.js-canonicalURLPath')?.dataset[
  'canonicalUrlPath'
];

document.addEventListener('keydown', e => {
  // TODO(golang.org/issue/40246): consolidate keyboard shortcut behavior across the site.
  const t = (e.target as HTMLElement)?.tagName;
  if (t === 'INPUT' || t === 'SELECT' || t === 'TEXTAREA') {
    return;
  }
  if ((e.target as HTMLElement)?.isContentEditable) {
    return;
  }
  if (e.metaKey || e.ctrlKey) {
    return;
  }
  switch (e.key) {
    // Temporary shortcut for testing out the dark theme.
    case 't': {
      let nextTheme = 'dark';
      const theme = document.documentElement.getAttribute('data-theme');
      if (theme === 'dark') {
        nextTheme = 'light';
      }
      document.documentElement.setAttribute('data-theme', nextTheme);
      break;
    }
    case 'y':
      if (canonicalURLPath && canonicalURLPath !== '') {
        window.history.replaceState(null, '', canonicalURLPath);
      }
      break;
    case '/':
      // Favoring the Firefox quick find feature over search input
      // focus. See: https://github.com/golang/go/issues/41093.
      if (searchInput && !window.navigator.userAgent.includes('Firefox')) {
        e.preventDefault();
        searchInput.focus();
      }
      break;
  }
});
