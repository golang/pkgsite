/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

// This file implements the behavior of the "jump to symbol" dialog for Go
// package documentation, as well as the simple dialog that displays keyboard
// shortcuts.

// The DOM for the dialogs is at the bottom of static/frontend/unit/main/_modals.tmpl.
// The CSS is in static/frontend/unit/main/_modals.css.

// The dialog is activated by pressing the 'f' key. It presents a list
// (#JumpDialog-list) of all Go symbols displayed in the documentation.
// Entering text in the dialog's text box (#JumpDialog-filter) restricts the
// list to symbols containing the text. Clicking on an symbol jumps to
// its documentation.

// This code is based on
// https://go.googlesource.com/gddo/+/refs/heads/master/gddo-server/assets/site.js.
// It was modified to remove the dependence on jquery and bootstrap.

const jumpDialog = document.querySelector<HTMLDialogElement>('.JumpDialog');
const jumpBody = jumpDialog?.querySelector<HTMLDivElement>('.JumpDialog-body');
const jumpList = jumpDialog?.querySelector<HTMLDivElement>('.JumpDialog-list');
const jumpFilter = jumpDialog?.querySelector<HTMLInputElement>('.JumpDialog-input');
const doc = document.querySelector<HTMLDivElement>('.js-documentation');

interface JumpListItem {
  link: HTMLAnchorElement;
  name: string;
  kind: string;
  lower: string;
}

let jumpListItems: JumpListItem[] | undefined; // All the symbols in the doc; computed only once.

// collectJumpListItems returns a list of items, one for each symbol in the
// documentation on the current page.
//
// It uses the data-kind attribute generated in the documentation HTML to find
// the symbols and their id attributes.
//
// If there are no data-kind attributes, then we have older doc; fall back to
// a less precise method.
function collectJumpListItems() {
  const items = [];
  if (!doc) return;
  for (const el of doc.querySelectorAll('[data-kind]')) {
    items.push(newJumpListItem(el));
  }

  // Clicking on any of the links closes the dialog.
  for (const item of items) {
    item.link.addEventListener('click', function () {
      jumpDialog?.close();
    });
  }
  // Sort case-insensitively by symbol name.
  items.sort(function (a, b) {
    return a.lower.localeCompare(b.lower);
  });
  return items;
}

// newJumpListItem creates a new item for the DOM element el.
// An item is an object with:
// - name: the element's id (which is the symbol name)
// - kind: the element's kind (function, variable, etc.),
// - link: a link ('a' tag) to the element
// - lower: the name in lower case, just for sorting
function newJumpListItem(el: Element): JumpListItem {
  const a = document.createElement('a');
  const name = el.getAttribute('id');
  a.setAttribute('href', '#' + name);
  a.setAttribute('tabindex', '-1');
  a.setAttribute('data-gtmc', 'jump to link');
  const kind = el.getAttribute('data-kind');
  return {
    link: a,
    name: name ?? '',
    kind: kind ?? '',
    lower: name?.toLowerCase() ?? '', // for sorting
  };
}

let lastFilterValue: string; // The last contents of the filter text box.
let activeJumpItem = -1; // The index of the currently active item in the list.

// updateJumpList sets the elements of the dialog list to
// everything whose name contains filter.
function updateJumpList(filter: string) {
  lastFilterValue = filter;
  if (!jumpListItems) {
    jumpListItems = collectJumpListItems();
  }
  setActiveJumpItem(-1);

  // Remove all children from list.
  while (jumpList?.firstChild) {
    jumpList.firstChild.remove();
  }

  if (filter) {
    // A filter is set. We treat the filter as a substring that can appear in
    // an item name (case insensitive), and find the following matches - in
    // order of priority:
    //
    // 1. Exact matches (the filter matches the item's name exactly)
    // 2. Prefix matches (the item's name starts with filter)
    // 3. Infix matches (the filter is a substring of the item's name)
    const filterLowerCase = filter.toLowerCase();

    const exactMatches = [];
    const prefixMatches = [];
    const infixMatches = [];

    // makeLinkHtml creates the link name HTML for a list item. item is the DOM
    // item. item.name.substr(boldStart, boldEnd) will be bolded.
    const makeLinkHtml = (item: JumpListItem, boldStart: number, boldEnd: number) => {
      return (
        item.name.substring(0, boldStart) +
        '<b>' +
        item.name.substring(boldStart, boldEnd) +
        '</b>' +
        item.name.substring(boldEnd)
      );
    };

    for (const item of jumpListItems ?? []) {
      const nameLowerCase = item.name.toLowerCase();

      if (nameLowerCase === filterLowerCase) {
        item.link.innerHTML = makeLinkHtml(item, 0, item.name.length);
        exactMatches.push(item);
      } else if (nameLowerCase.startsWith(filterLowerCase)) {
        item.link.innerHTML = makeLinkHtml(item, 0, filter.length);
        prefixMatches.push(item);
      } else {
        const index = nameLowerCase.indexOf(filterLowerCase);
        if (index > -1) {
          item.link.innerHTML = makeLinkHtml(item, index, index + filter.length);
          infixMatches.push(item);
        }
      }
    }

    for (const item of exactMatches.concat(prefixMatches).concat(infixMatches)) {
      jumpList?.appendChild(item.link);
    }
  } else {
    if (!jumpListItems || jumpListItems.length === 0) {
      const msg = document.createElement('i');
      msg.innerHTML = 'There are no symbols on this page.';
      jumpList?.appendChild(msg);
    }
    // No filter set; display all items in their existing order.
    for (const item of jumpListItems ?? []) {
      item.link.innerHTML = item.name + ' <i>' + item.kind + '</i>';
      jumpList?.appendChild(item.link);
    }
  }

  if (jumpBody) {
    jumpBody.scrollTop = 0;
  }
  if (jumpListItems?.length && jumpList && jumpList.children.length > 0) {
    setActiveJumpItem(0);
  }
}

// Set the active jump item to n.
function setActiveJumpItem(n: number) {
  const cs = jumpList?.children as HTMLCollectionOf<HTMLElement> | null | undefined;
  if (!cs || !jumpBody) {
    return;
  }
  if (activeJumpItem >= 0) {
    cs[activeJumpItem].classList.remove('JumpDialog-active');
  }
  if (n >= cs.length) {
    n = cs.length - 1;
  }
  if (n >= 0) {
    cs[n].classList.add('JumpDialog-active');

    // Scroll so the active item is visible.
    // For some reason cs[n].scrollIntoView() doesn't behave as I'd expect:
    // it moves the entire dialog box in the viewport.

    // Get the top and bottom of the active item relative to jumpBody.
    const activeTop = cs[n].offsetTop - cs[0].offsetTop;
    const activeBottom = activeTop + cs[n].clientHeight;
    if (activeTop < jumpBody.scrollTop) {
      // Off the top; scroll up.
      jumpBody.scrollTop = activeTop;
    } else if (activeBottom > jumpBody.scrollTop + jumpBody.clientHeight) {
      // Off the bottom; scroll down.
      jumpBody.scrollTop = activeBottom - jumpBody.clientHeight;
    }
  }
  activeJumpItem = n;
}

// Increment the activeJumpItem by delta.
function incActiveJumpItem(delta: number) {
  if (activeJumpItem < 0) {
    return;
  }
  let n = activeJumpItem + delta;
  if (n < 0) {
    n = 0;
  }
  setActiveJumpItem(n);
}

// Pressing a key in the filter updates the list (if the filter actually changed).
jumpFilter?.addEventListener('keyup', function () {
  if (jumpFilter.value.toUpperCase() != lastFilterValue.toUpperCase()) {
    updateJumpList(jumpFilter.value);
  }
});

// Pressing enter in the filter selects the first element in the list.
jumpFilter?.addEventListener('keydown', function (event) {
  const upArrow = 38;
  const downArrow = 40;
  const enterKey = 13;
  switch (event.which) {
    case upArrow:
      incActiveJumpItem(-1);
      event.preventDefault();
      break;
    case downArrow:
      incActiveJumpItem(1);
      event.preventDefault();
      break;
    case enterKey:
      if (activeJumpItem >= 0) {
        if (jumpList) {
          (jumpList.children[activeJumpItem] as HTMLElement).click();
          event.preventDefault();
        }
      }
      break;
  }
});

const shortcutsDialog = document.querySelector<HTMLDialogElement>('.ShortcutsDialog');

// Keyboard shortcuts:
// - Pressing '/' focuses the search box
// - Pressing 'f' or 'F' opens the jump-to-symbol dialog.
// - Pressing '?' opens up the shortcut dialog.
// Ignore a keypress if a dialog is already open, or if it is pressed on a
// component that wants to consume it.
document.addEventListener('keypress', function (e) {
  if (jumpDialog?.open || shortcutsDialog?.open) {
    return;
  }
  const target = e.target as HTMLElement | null;
  const t = target?.tagName;
  if (t == 'INPUT' || t == 'SELECT' || t == 'TEXTAREA') {
    return;
  }
  if (target?.contentEditable == 'true') {
    return;
  }
  if (e.metaKey || e.ctrlKey) {
    return;
  }
  const ch = String.fromCharCode(e.which);
  switch (ch) {
    case 'f':
    case 'F':
      e.preventDefault();
      if (jumpFilter) {
        jumpFilter.value = '';
      }
      jumpDialog?.showModal();
      jumpFilter?.focus();
      updateJumpList('');
      break;
    case '?':
      shortcutsDialog?.showModal();
      break;
  }
});

const jumpOutlineInput = document.querySelector('.js-jumpToInput');
if (jumpOutlineInput) {
  jumpOutlineInput.addEventListener('click', () => {
    if (jumpFilter) {
      jumpFilter.value = '';
    }
    updateJumpList('');
  });
}
