/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { AccordionController } from './accordion.js';

/**
 * Instantiates accordion controller for the left sidebar.
 */
const accordion = document.querySelector('.js-accordion');
if (accordion) {
  new AccordionController(accordion);
}

/**
 * Event handlers for expanding and collapsing the readme section.
 */
const readme = document.querySelector('.js-readme');
const readmeContent = document.querySelector('.js-readmeContent');
const readmeExpand = document.querySelectorAll('.js-readmeExpand');
const readmeCollapse = document.querySelector('.js-readmeCollapse');
if (readme && readmeContent && readmeExpand.length && readmeCollapse) {
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
    readmeExpand[1].scrollIntoView({ block: 'center' });
  });
  readmeContent.addEventListener('keyup', e => {
    readme.classList.add('UnitReadme--expanded');
  });
}

/**
 * Disable unavailable sections in navigation dropdown on mobile.
 */
const readmeOption = document.querySelector('.js-readmeOption');
if (!readme) {
  readmeOption.setAttribute('disabled', true);
}

const unitFiles = document.querySelector('.js-unitFiles');
const filesOption = document.querySelector('.js-filesOption');
if (!unitFiles) {
  filesOption.setAttribute('disabled', true);
}

const unitDirectories = document.querySelector('.js-unitDirectories');
const directoriesOption = document.querySelector('.js-directoriesOption');
if (!unitDirectories) {
  directoriesOption.setAttribute('disabled', true);
}
