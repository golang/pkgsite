/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { analytics, keyboard } from '../shared/shared';

// Temporary shortcut for testing out the dark theme.
keyboard.on('t', 'toggle theme', () => {
  let nextTheme = 'dark';
  const theme = document.documentElement.getAttribute('data-theme');
  if (theme === 'dark') {
    nextTheme = 'light';
  }
  document.documentElement.setAttribute('data-theme', nextTheme);
});

// Pressing '/' focuses the search box
keyboard.on('/', 'focus search', e => {
  const searchInput = Array.from(
    document.querySelectorAll<HTMLInputElement>('.js-searchFocus')
  ).pop();
  // Favoring the Firefox quick find feature over search input
  // focus. See: https://github.com/golang/go/issues/41093.
  if (searchInput && !window.navigator.userAgent.includes('Firefox')) {
    e.preventDefault();
    searchInput.focus();
  }
});

// Pressing 'y' changes the browser URL to the canonical URL
// without triggering a reload.
keyboard.on('y', 'set canonical url', () => {
  const canonicalURLPath = document.querySelector<HTMLDivElement>('.js-canonicalURLPath')?.dataset[
    'canonicalUrlPath'
  ];
  if (canonicalURLPath && canonicalURLPath !== '') {
    window.history.replaceState(null, '', canonicalURLPath);
  }
});

/**
 * setupGoogleTagManager intializes Google Tag Manager.
 */
(function setupGoogleTagManager() {
  analytics.track({
    'gtm.start': new Date().getTime(),
    event: 'gtm.js',
  });
})();

/**
 * removeUTMSource removes the utm_source GET parameter if present.
 * This is done using JavaScript, so that the utm_source is still
 * captured by Google Analytics.
 */
function removeUTMSource() {
  const urlParams = new URLSearchParams(window.location.search);
  const utmSource = urlParams.get('utm_source');
  if (utmSource !== 'gopls' && utmSource !== 'godoc' && utmSource !== 'pkggodev') {
    return;
  }

  /** Strip the utm_source query parameter and replace the URL. **/
  const newURL = new URL(window.location.href);
  urlParams.delete('utm_source');
  newURL.search = urlParams.toString();
  window.history.replaceState(null, '', newURL.toString());
}

if (document.querySelector<HTMLElement>('.js-gtmID')?.dataset.gtmid && window.dataLayer) {
  analytics.func(function () {
    removeUTMSource();
  });
} else {
  removeUTMSource();
}
