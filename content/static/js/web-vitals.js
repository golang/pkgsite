/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  getCLS,
  getFID,
  getLCP,
  getFCP,
  getTTFB,
} from '/third_party/web-vitals/web-vitals.es5.min.js?module';

// This code is based on
// https://github.com/GoogleChrome/web-vitals/tree/master#send-the-results-to-google-tag-manager
// and sends web vitals metrics to google tag manager.
function sendToGTM({ name, delta, id }) {
  window.dataLayer = window.dataLayer || [];
  window.dataLayer.push({
    event: 'web-vitals',
    event_category: 'Web Vitals',
    event_action: name,
    event_value: Math.round(name === 'CLS' ? delta * 1000 : delta),
    event_label: id,
  });
}

// Cumulative Layout Shift (https://web.dev/cls/)
getCLS(sendToGTM);

// First Input Delay (https://web.dev/fid/)
getFID(sendToGTM);

// Largest Contentful Paint (https://web.dev/lcp/)
getLCP(sendToGTM);

// First Contentful Paint (https://web.dev/fcp/)
getFCP(sendToGTM);

// Time to First Byte: (https://web.dev/time-to-first-byte/)
getTTFB(sendToGTM);
