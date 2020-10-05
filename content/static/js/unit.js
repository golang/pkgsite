/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

// Highlights the active accordian on page load and adds event
// listeners to update on interaction from user.
document.querySelectorAll('a.js-accordian').forEach((anchor, index) => {
  const activeClass = 'UnitOutline-accordian--active';
  if (index === 0) {
    anchor.classList.add(activeClass);
  }
  anchor.addEventListener('click', () => {
    document.querySelectorAll('a.js-accordian').forEach(el => {
      el.classList.remove(activeClass);
    });
    anchor.classList.add(activeClass);
  });
});

/**
 * Event handlers for expanding and collapsing the readme section.
 */
const readme = document.querySelector('.js-readme');
const readmeExpand = document.querySelectorAll('.js-readmeExpand');
const readmeCollapse = document.querySelector('.js-readmeCollapse');
if (readmeExpand && readmeExpand && readmeCollapse) {
  readmeExpand.forEach(el =>
    el.addEventListener('click', e => {
      e.preventDefault();
      readme.classList.add('UnitReadme--expanded');
      readme.scrollIntoView();
    })
  );
  readmeCollapse.addEventListener('click', e => {
    e.preventDefault();
    readme.classList.remove('UnitReadme--expanded');
    readme.scrollIntoView();
  });
}
