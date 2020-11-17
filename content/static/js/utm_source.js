/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * removeUTMSource removes the utm_source GET parameter if present.
 * This is done using JavaScript, so that the utm_source is still
 * captured by Google Analytics.
 */
window.onload = event => {
  var urlParams = new URLSearchParams(window.location.search);
  var utmSource = urlParams.get('utm_source');
  if (utmSource !== 'gopls' && utmSource !== 'godoc') {
    return;
  }

  /** Strip the utm_source query parameter and replace the URL. **/
  var newURL = new URL(window.location.href);
  urlParams.delete('utm_source');
  newURL.search = urlParams.toString();
  window.history.replaceState(null, '', newURL.toString());
};
