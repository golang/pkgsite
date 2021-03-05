/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

const fetchButton = document.querySelector('.js-fetchButton');
if (fetchButton) {
  fetchButton.addEventListener('click', e => {
    e.preventDefault();
    fetchPath();
  });
}

async function fetchPath() {
  const fetchMessageEl = document.querySelector<HTMLHeadingElement>('.js-fetchMessage');
  const fetchMessageSecondary = document.querySelector<HTMLParagraphElement>(
    '.js-fetchMessageSecondary'
  );
  const fetchButton = document.querySelector<HTMLButtonElement>('.js-fetchButton');
  const fetchLoading = document.querySelector<HTMLDivElement>('.js-fetchLoading');
  if (!(fetchMessageEl && fetchMessageSecondary && fetchButton && fetchLoading)) {
    return;
  }
  fetchMessageEl.textContent = `Fetching ${fetchMessageEl.dataset.path}`;
  fetchMessageSecondary.textContent =
    'Feel free to navigate away and check back later, we’ll keep working on it!';
  fetchButton.style.display = 'none';
  fetchLoading.style.display = 'block';

  const response = await fetch(`/fetch${window.location.pathname}`, { method: 'POST' });
  if (response.ok) {
    window.location.reload();
    return;
  }
  const responseText = await response.text();
  fetchLoading.style.display = 'none';
  fetchMessageSecondary.textContent = '';
  const responseTextParsedDOM = new DOMParser().parseFromString(responseText, 'text/html');
  fetchMessageEl.innerHTML = responseTextParsedDOM.documentElement.textContent ?? '';
}
