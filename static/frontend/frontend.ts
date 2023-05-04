/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { registerHeaderListeners, registerSearchFormListeners } from 'static/shared/header/header';
import { CarouselController } from 'static/shared/carousel/carousel';
import { ClipboardController } from 'static/shared/clipboard/clipboard';
import { ToolTipController } from 'static/shared/tooltip/tooltip';
import { SelectNavController } from 'static/shared/outline/select';
import { ModalController } from 'static/shared/modal/modal';
import { initModals } from 'static/shared/jump/jump';

import { keyboard } from 'static/shared/keyboard/keyboard';
import * as analytics from 'static/shared/analytics/analytics';
import { initJumpLinks } from './about/index';

window.addEventListener('load', () => {
  for (const el of document.querySelectorAll<HTMLButtonElement>('.js-clipboard')) {
    new ClipboardController(el);
  }

  for (const el of document.querySelectorAll<HTMLDialogElement>('.js-modal')) {
    new ModalController(el);
  }

  for (const t of document.querySelectorAll<HTMLDetailsElement>('.js-tooltip')) {
    new ToolTipController(t);
  }

  for (const el of document.querySelectorAll<HTMLSelectElement>('.js-selectNav')) {
    new SelectNavController(el);
  }

  for (const el of document.querySelectorAll<HTMLSelectElement>('.js-carousel')) {
    new CarouselController(el);
  }

  for (const el of document.querySelectorAll('.js-toggleTheme')) {
    el.addEventListener('click', () => {
      toggleTheme();
    });
  }

  if (document.querySelector<HTMLElement>('.js-gtmID')?.dataset.gtmid && window.dataLayer) {
    analytics.func(function () {
      removeUTMSource();
    });
  } else {
    removeUTMSource();
  }

  registerHeaderListeners();
  registerSearchFormListeners();
  initModals();
  initJumpLinks();
  registerCookieNotice();
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
  let canonicalURLPath = document.querySelector<HTMLDivElement>('.js-canonicalURLPath')?.dataset[
    'canonicalUrlPath'
  ];
  if (canonicalURLPath && canonicalURLPath !== '') {
    const fragment = window.location.hash;
    if (fragment) {
      canonicalURLPath += fragment;
    }
    window.history.replaceState(null, '', canonicalURLPath);
  }
});

/**
 * setupGoogleTagManager initializes Google Tag Manager.
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

/**
 * toggleTheme switches the preferred color scheme between auto, light, and dark.
 */
function toggleTheme() {
  let nextTheme = 'dark';
  const theme = document.documentElement.getAttribute('data-theme');
  if (theme === 'dark') {
    nextTheme = 'light';
  } else if (theme === 'light') {
    nextTheme = 'auto';
  }
  let domain = '';
  if (location.hostname.endsWith('go.dev')) {
    domain = 'domain=.go.dev;';
  }
  document.documentElement.setAttribute('data-theme', nextTheme);
  document.cookie = `prefers-color-scheme=${nextTheme};${domain}path=/;max-age=31536000;`;
}

/**
 * registerCookieNotice makes the cookie notice visible and adds listeners to dismiss it
 * if it has not yet been acknowledge by the user.
 */
function registerCookieNotice() {
  const themeCookie = document.cookie.match(/cookie-consent=true/);
  if (!themeCookie) {
    const notice = document.querySelector('.js-cookieNotice');
    const button = notice?.querySelector('button');
    notice?.classList.add('Cookie-notice--visible');
    button?.addEventListener('click', () => {
      let domain = '';
      if (location.hostname.endsWith('go.dev')) {
        // Apply the cookie to *.go.dev.
        domain = 'domain=.go.dev;';
      }
      document.cookie = `cookie-consent=true;${domain}path=/;max-age=31536000`;
      notice?.remove();
    });
  }
}
