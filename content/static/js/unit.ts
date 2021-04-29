/*!
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { CopyToClipboardController } from './clipboard.js';
import './toggle-tip.js';
import { ExpandableRowsTableController } from './table.js';

document
  .querySelectorAll<HTMLTableElement>('.js-expandableTable')
  .forEach(
    el =>
      new ExpandableRowsTableController(
        el,
        document.querySelector<HTMLButtonElement>('.js-expandAllDirectories')
      )
  );

/**
 * Instantiates CopyToClipboardController controller copy buttons
 * on the unit page.
 */
document.querySelectorAll<HTMLButtonElement>('.js-copyToClipboard').forEach(el => {
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
const mobileNavSelect = document.querySelector<HTMLSelectElement>('.DocNavMobile-select');
if (readme && readmeContent && readmeOutline && readmeExpand.length && readmeCollapse) {
  if (window.location.hash.includes('readme')) {
    readme.classList.add('UnitReadme--expanded');
  }
  mobileNavSelect?.addEventListener('change', e => {
    if ((e.target as HTMLSelectElement).value.startsWith('readme-')) {
      readme.classList.add('UnitReadme--expanded');
    }
  });
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
    if (readmeExpand[1]) {
      readmeExpand[1].scrollIntoView({ block: 'center' });
    }
  });
  readmeContent.addEventListener('keyup', () => {
    readme.classList.add('UnitReadme--expanded');
  });
  readmeContent.addEventListener('click', () => {
    readme.classList.add('UnitReadme--expanded');
  });
  readmeOutline.addEventListener('click', () => {
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
  readmeOption.setAttribute('disabled', 'true');
}
const unitDirectories = document.querySelector('.js-unitDirectories');
const directoriesOption = document.querySelector('.js-directoriesOption');
if (!unitDirectories && directoriesOption) {
  directoriesOption.setAttribute('disabled', 'true');
}
document.querySelectorAll('.js-buildContextSelect').forEach(el => {
  el.addEventListener('change', e => {
    window.location.search = `?GOOS=${(e.target as HTMLSelectElement).value}`;
  });
});

/**
 * Adds double click event listener to the header that will
 * scroll the page back to the top.
 */
const unitHeader = document.querySelector('.js-unitHeader');
unitHeader?.addEventListener('dblclick', e => {
  const target = e.target as HTMLElement;
  if (
    target === unitHeader.firstElementChild &&
    unitHeader.classList.contains('UnitHeader--sticky')
  ) {
    window.getSelection()?.removeAllRanges();
    window.scrollTo({ top: 0 });
  }
});

/**
 * Calculates dynamic heights values for header elements to support
 * variable size sticky positioned elements in the header so that banners
 * and breadcumbs may overflow to multiple lines.
 */
const header = document.querySelector<HTMLElement>('.UnitHeader');
const breadcrumbs = header?.querySelector<HTMLElement>('.UnitHeader-breadcrumbs');
const content = header?.querySelector<HTMLElement>('.UnitHeader-content');
const calcSize = () => {
  document.documentElement.style.removeProperty('--full-header-height');
  document.documentElement.style.setProperty(
    '--full-header-height',
    `${(header?.getBoundingClientRect().height ?? 0) / 16}rem`
  );
  document.documentElement.style.setProperty('--banner-height', `0rem`);
  document.documentElement.style.setProperty(
    '--breadcrumbs-height',
    `${(breadcrumbs?.getBoundingClientRect().height ?? 0) / 16}rem`
  );
  document.documentElement.style.setProperty(
    '--content-height',
    `${(content?.getBoundingClientRect().height ?? 0) / 16}rem`
  );
};
calcSize();
window.addEventListener('resize', function () {
  calcSize();
});

/**
 * Observer for header that applies classnames to transition
 * header elements into their sticky position and back into the
 * full size position.
 */
const observer = new IntersectionObserver(
  ([e]) => {
    if (e.intersectionRatio < 1) {
      unitHeader?.classList.add('UnitHeader--sticky');
      unitHeader?.classList.remove('UnitHeader--full');
    } else {
      unitHeader?.classList.remove('UnitHeader--sticky');
      unitHeader?.classList.add('UnitHeader--full');
      calcSize();
    }
  },
  { threshold: 1.0, rootMargin: '40px' }
);

const headerSentinel = document.querySelector('.js-headerSentinel');
if (headerSentinel) {
  observer.observe(headerSentinel);
}
