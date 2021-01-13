/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { CopyToClipboardController } from './clipboard.js';
import './toggle-tip.js';
import { ExpandableRowsTableController } from '/static/js/table.js';

document
  .querySelectorAll('.js-expandableTable')
  .forEach(
    el => new ExpandableRowsTableController(el, document.querySelector('.js-expandAllDirectories'))
  );

/**
 * Instantiates CopyToClipboardController controller copy buttons
 * on the unit page.
 */
document.querySelectorAll('.js-copyToClipboard').forEach(el => {
  new CopyToClipboardController(el);
});

/**
 * Event handlers for expanding and collapsing the readme section.
 */
const readme = document.querySelector('.js-readme');
const readmeContent = document.querySelector('.js-readmeContent');
const readmeOutline = document.querySelector('.js-readmeOutline');
const readmeExpand = document.querySelectorAll('.js-readmeExpand');
const readmeCollapse = document.querySelector('.js-readmeCollapse');
if (readme && readmeContent && readmeOutline && readmeExpand.length && readmeCollapse) {
  if (window.location.hash.includes('readme')) {
    readme.classList.add('UnitReadme--expanded');
  }
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
  readmeContent.addEventListener('click', e => {
    readme.classList.add('UnitReadme--expanded');
  });
  readmeOutline.addEventListener('click', e => {
    readme.classList.add('UnitReadme--expanded');
  });
  document.addEventListener('keydown', e => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
      readme.classList.add('UnitReadme--expanded');
    }
  });
}

/**
 * Disable unavailable sections in navigation dropdown on mobile.
 */
const readmeOption = document.querySelector('.js-readmeOption');
if (readmeOption && !readme) {
  readmeOption.setAttribute('disabled', true);
}
const unitDirectories = document.querySelector('.js-unitDirectories');
const directoriesOption = document.querySelector('.js-directoriesOption');
if (!unitDirectories) {
  directoriesOption.setAttribute('disabled', true);
}
