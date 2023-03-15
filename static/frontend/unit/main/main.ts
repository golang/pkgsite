import { initPlaygrounds } from 'static/shared/playground/playground';
import { SelectNavController, makeSelectNav } from 'static/shared/outline/select';
import { TreeNavController } from 'static/shared/outline/tree';
import { ExpandableRowsTableController } from 'static/shared/table/table';

initPlaygrounds();

const directories = document.querySelector<HTMLTableElement>('.js-expandableTable');
if (directories) {
  const table = new ExpandableRowsTableController(
    directories,
    document.querySelector<HTMLButtonElement>('.js-expandAllDirectories')
  );
  // Expand directories on page load with expand-directories query param.
  if (window.location.search.includes('expand-directories')) {
    table.expandAllItems();
  }

  const internalToggle = document.querySelector<HTMLButtonElement>('.js-showInternalDirectories');
  if (internalToggle) {
    if (document.querySelector('.UnitDirectories-internal')) {
      internalToggle.style.display = 'block';
    }
    internalToggle.addEventListener('click', () => {
      if (directories.classList.contains('UnitDirectories-showInternal')) {
        directories.classList.remove('UnitDirectories-showInternal');
        internalToggle.innerText = 'Show internal';
      } else {
        directories.classList.add('UnitDirectories-showInternal');
        internalToggle.innerText = 'Hide internal';
      }
    });
  }
  if (document.querySelector('html[data-local="true"]')) {
    internalToggle?.click();
  }
}

const treeEl = document.querySelector<HTMLElement>('.js-tree');
if (treeEl) {
  const treeCtrl = new TreeNavController(treeEl);
  const select = makeSelectNav(treeCtrl);
  const mobileNav = document.querySelector('.js-mainNavMobile');
  if (mobileNav && mobileNav.firstElementChild) {
    mobileNav?.replaceChild(select, mobileNav.firstElementChild);
  }
  if (select.firstElementChild) {
    new SelectNavController(select.firstElementChild);
  }
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
  if (readme.clientHeight > 320) {
    readme?.classList.remove('UnitReadme--expanded');
    readme?.classList.add('UnitReadme--toggle');
  }
  if (window.location.hash.includes('readme')) {
    expandReadme();
  }
  mobileNavSelect?.addEventListener('change', e => {
    if ((e.target as HTMLSelectElement).value.startsWith('readme-')) {
      expandReadme();
    }
  });
  readmeExpand.forEach(el =>
    el.addEventListener('click', e => {
      e.preventDefault();
      expandReadme();
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
    expandReadme();
  });
  readmeContent.addEventListener('click', () => {
    expandReadme();
  });
  readmeOutline.addEventListener('click', () => {
    expandReadme();
  });
  document.addEventListener('keydown', e => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
      expandReadme();
    }
  });
}

/**
 * expandReadme expands the readme and adds the section-readme hash to the
 * URL so it stays expanded when navigating back from an external link.
 */
function expandReadme() {
  history.replaceState(null, '', `${location.pathname}#section-readme`);
  readme?.classList.add('UnitReadme--expanded');
}

/**
 * Expand details items that are focused. This will expand
 * deprecated symbols when they are navigated to from the index
 * or a direct link.
 */
function openDeprecatedSymbol() {
  if (!location.hash) return;
  const heading = document.getElementById(location.hash.slice(1));
  const grandParent = heading?.parentElement?.parentElement as HTMLDetailsElement | null;
  if (grandParent?.nodeName === 'DETAILS') {
    grandParent.open = true;
  }
}
openDeprecatedSymbol();
window.addEventListener('hashchange', () => openDeprecatedSymbol());

/**
 * Listen for changes in the build context dropdown.
 */
document.querySelectorAll('.js-buildContextSelect').forEach(el => {
  el.addEventListener('change', e => {
    window.location.search = `?GOOS=${(e.target as HTMLSelectElement).value}`;
  });
});
