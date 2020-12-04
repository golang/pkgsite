/**
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

const snippetEls = document.querySelectorAll('.js-toolsCopySnippet');
snippetEls.forEach(inputEl => {
  inputEl.addEventListener('click', e => {
    e.preventDefault();
    e.currentTarget.select();
    document.execCommand('copy');
  });
});

const pathEl = document.querySelector('.js-toolsPathInput');
const htmlEl = document.querySelector('input[name="html"].js-toolsCopySnippet');
const markdownEl = document.querySelector('input[name="markdown"].js-toolsCopySnippet');
const badgeEl = document.querySelector('.js-badgeExampleButton');
if (pathEl && htmlEl && markdownEl && badgeEl) {
  pathEl.addEventListener('input', e => {
    const origin = window.location.origin;
    const href = `${origin}/${e.target.value}`;
    const imgSrc = `${origin}/badge/${e.target.value}`;
    htmlEl.value = `<a href="${href}"><img src="${imgSrc}" alt="Go Reference"></a>`;
    markdownEl.value = `[![Go Reference](${imgSrc})](${href})`;
    badgeEl.href = href;
  });
}
