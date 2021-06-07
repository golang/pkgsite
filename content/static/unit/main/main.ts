import { SelectNavController, makeSelectNav } from '../../outline/select.js';
import { TreeNavController } from '../../outline/tree.js';

import '../../jump/jump.js';
import '../../playground/playground.js';
import { ExpandableRowsTableController } from '../../table/table.js';

const el = <T extends HTMLElement>(selector: string) => document.querySelector<T>(selector);

const treeCtrl = new TreeNavController(el('.js-tree'));
const select = makeSelectNav(treeCtrl);
const mobileNav = document.querySelector('.js-mainNavMobile');
mobileNav.replaceChild(select, mobileNav.firstElementChild);
new SelectNavController(select.firstElementChild);

const expandableTable = el<HTMLTableElement>('.js-expandableTable');
if (expandableTable) {
  new ExpandableRowsTableController(expandableTable);
}

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
 * Expand details items that are focused. This will expand
 * deprecated symbols when they are navigated to from the index
 * or a direct link.
 */
for (const a of document.querySelectorAll<HTMLAnchorElement>('.js-deprecatedTagLink')) {
  const hash = new URL(a.href).hash;
  const heading = document.querySelector(hash);
  const details = heading?.parentElement?.parentElement as HTMLDetailsElement | null;
  if (details) {
    a.addEventListener('click', () => {
      details.open = true;
    });
    if (location.hash === hash) {
      details.open = true;
    }
  }
}

/**
 * Listen for changes in the build context dropdown.
 */
document.querySelectorAll('.js-buildContextSelect').forEach(el => {
  el.addEventListener('change', e => {
    window.location.search = `?GOOS=${(e.target as HTMLSelectElement).value}`;
  });
});
