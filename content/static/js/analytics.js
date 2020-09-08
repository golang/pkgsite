/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

(function setupGoogleTagManager() {
  window.dataLayer = window.dataLayer || [];
  window.dataLayer.push({
    'gtm.start': new Date().getTime(),
    event: 'gtm.js',
  });
})();

/**
 * trackErrors creates an event listener that reports unhandled exceptions
 * to Google Tag Manager.
 */
(function trackErrors() {
  const loadErrorEvents = (window.__err && window.__err.p) || [];
  const trackError = error => {
    window.dataLayer.push({
      event: 'error',
      event_category: 'Script',
      event_action: 'uncaught error',
      event_label: (error && (error.stack || `${error.name}: ${error.message}`)) || '(not set)',
    });
  };

  for (let event of loadErrorEvents) {
    trackError(event.error);
  }

  window.addEventListener('error', event => {
    trackError(event.error);
  });
})();

/**
 * trackRejections creates an event listener that reports unhandled
 * promise rejections to Google Tag Manager.
 */
(function trackRejections() {
  const rejectionEvents = new Map();
  const reportUnhandledRejections = () => {
    for (var reason of rejectionEvents.values()) {
      window.dataLayer.push({
        event: 'error',
        event_category: 'Script',
        event_action: 'unhandled rejection',
        event_label: (reason && (reason.stack || reason.message)) || '(not set)',
      });
    }
    rejectionEvents.clear();
  };

  window.addEventListener('unhandledrejection', event => {
    rejectionEvents.set(event.promise, event.reason);
    // Checking for requestIdleCallback compatibility and
    // falling back to setTimeout with arbitrary timeout
    // of 250ms.
    if (typeof window.requestIdleCallback !== 'undefined') {
      window.requestIdleCallback(() => {
        reportUnhandledRejections();
      }, {timeout: 1000});
    } else {
      window.setTimeout(() => {
        reportUnhandledRejections();
      }, 250);
    }
  });

  window.addEventListener('rejectionhandled', event => {
    rejectionEvents.delete(event.promise);
  });
})();
