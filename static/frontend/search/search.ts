/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

const hiddenChips = document.querySelectorAll('.js-sameModChip[data-hidden]');
const showMore = document.querySelector<HTMLButtonElement>('.js-showMoreChip');
const searchLimit = document.querySelector<HTMLSelectElement>('.js-searchLimit');

showMore?.addEventListener('click', () => {
  for (const el of hiddenChips) {
    el.removeAttribute('data-hidden');
  }
  showMore.parentElement?.removeChild(showMore);
});

searchLimit?.addEventListener('change', e => {
  const target = e.target as HTMLElement;
  const form = target.parentNode as HTMLFormElement;
  form.submit();
});
