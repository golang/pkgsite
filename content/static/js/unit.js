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
