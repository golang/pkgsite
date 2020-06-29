/**
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
  const fetchMessageEl = document.querySelector('.js-fetchMessage');
  fetchMessageEl.textContent = `Fetching ${fetchMessageEl.dataset.path}`;
  document.querySelector('.js-fetchMessageSecondary').textContent =
    "Feel free to navigate away and check back later, we’ll keep working on it!";
  document.querySelector('.js-fetchButton').style.display = 'none';
  document.querySelector('.js-fetchLoading').style.display = 'block';

  const response = await fetch(`/fetch${window.location.pathname}`);
  if (response.ok) {
    window.location.reload();
    return;
  }
  const responseText = await response.text();
  document.querySelector('.js-fetchLoading').style.display = 'none';
  document.querySelector('.js-fetchMessageSecondary').textContent = '';
  document.querySelector('.js-fetchMessage').textContent = responseText;
}
