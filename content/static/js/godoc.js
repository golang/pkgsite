/*
  Copyright 2019 The Go Authors. All rights reserved.
  Use of this source code is governed by a BSD-style
  license that can be found in the LICENSE file.
*/

if (window.location.hash !== '') {
  showHidden(window.location.hash.slice(1));
}
window.addEventListener('hashchange', () => {
  showHidden(window.location.hash.slice(1));
});

for (const e of document.querySelectorAll('.example')) {
  const a = e.querySelector('.example-header a');
  const body = e.querySelector('.example-body');
  a.addEventListener('click', event => {
    event.preventDefault();
    body.style.display = body.style.display === 'block' ? 'none' : 'block';
  });
}

function showHidden(id) {
  const body = document.querySelector(`#${id} .example-body`);
  if (body === null) {
    return;
  }
  body.style.display = 'block';
}
