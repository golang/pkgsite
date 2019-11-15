// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file implements the behavior of the "jump to identifer" dialog for Go
// package documentation, as well as the simple dialog that displays keyboard
// shortcuts.

// The DOM for the dialogs is at the bottom of content/static/html/pages/pkg_doc.tmpl.
// The CSS is in content/static/css/stylesheet.css.

// The dialog is activated by pressing the 'f' key. It presents a list
// (#JumpDialog-list) of all Go identifiers displayed in the documentation.
// Entering text in the dialog's text box (#JumpDialog-filter) restricts the
// list to identifiers containing the text. Clicking on an identifier jumps to
// its documentation.

// This code is based on
// https://go.googlesource.com/gddo/+/refs/heads/master/gddo-server/assets/site.js.
// It was modified to remove the dependence on jquery and bootstrap.

const jumpDialog = document.querySelector('.JumpDialog');
const jumpBody = jumpDialog.querySelector('.JumpDialog-body');
const jumpList = jumpDialog.querySelector('.JumpDialog-list');
const jumpFilter = jumpDialog.querySelector('.JumpDialog-input');

if (!jumpDialog.showModal) {
  dialogPolyfill.registerDialog(jumpDialog);
}

let jumpListItems; // All the identifiers in the doc; computed only once.

// collectJumpListItems returns a list of items, one for each identifier in the
// documentation on the current page.
function collectJumpListItems() {
  const items = [];
  // A map from id to bool, to dedup DOM ids. The doc DOM has duplicate ids (b/143456059).
  // We assume the first one is the one we want.
  const seen = {};
  // Attempt to find the relevant elements by looking through every element in the
  // .Documentation DOM that has an id attribute of a certain form.
  // TODO(b/143456714) Put a data-kind attribute on each relevant element, so this would
  // be more precise.
  const doc = document.querySelector('.Documentation');
  for (const el of doc.querySelectorAll('*[id]')) {
    const id = el.getAttribute('id');
    if (!seen[id] && /^[^_][^-]*$/.test(id)) {
      seen[id] = true;
      items.push(newJumpListItem(el));
    }
  }
  // Clicking on any of the links closes the dialog.
  for (const item of items) {
    item.link.addEventListener('click', function() { jumpDialog.close(); });
  }
  // Sort case-insensitively by identifier name.
  items.sort(function (a, b) { return a.lower.localeCompare(b.lower); });
  return items;
}


// newJumpListItem creates a new item for the DOM element el.
// An item is an object with:
// - name: the element's id (which is the identifer name)
// - kind: the element's kind (function, variable, etc.),
// - link: a link ('a' tag) to the element
// - lower: the name in lower case, just for sorting
function newJumpListItem(el) {
    const a = document.createElement('a');
    const name = el.getAttribute('id');
    a.setAttribute('href', '#' + name);
    a.setAttribute('tabindex', '-1');
    return {
        link: a,
        name: name,
        kind: guessKind(el),
        lower: name.toLowerCase(), // for sorting
    };
}

// guessKind tries to guess the kind of el by looking around the DOM.
// Fixing b/143456714 would make this unnecessary.
function guessKind(el) {
  switch (el.getAttribute('class')) {
    case 'Documentation-functionHeader':
    case 'Documentation-typeFuncHeader':
      return 'function';
    case 'Documentation-typeHeader':
      return 'type';
    case 'Documentation-typeMethodHeader':
      return 'method';
    default:
      const sec = el.closest('section');
      switch (sec.getAttribute('class')) {
        case 'Documentation-variables':
          return 'variable';
        case 'Documentation-constants':
          return 'constant';
	case 'Documentation-types':
          return 'field';
        default:
          return '';
      }
  }
}

let lastFilterValue;    // The last contents of the filter text box.
let activeJumpItem = -1;     // The index of the currently active item in the list.

// updateJumpList sets the elements of the dialog list to
// everything whose name contains filter.
function updateJumpList(filter) {
  lastFilterValue = filter;
  if (!jumpListItems) {
    jumpListItems = collectJumpListItems();
  }
  setActiveJumpItem(-1);

  // Remove all children from list.
  while (jumpList.firstChild) {
    jumpList.firstChild.remove();
  }
  // Make a regexp corresponding to filter. The result will match any string
  // containing filter, case-insensitively. Escape the regexp metacharacters in
  // filter.
  const re = new RegExp(filter.replace(/([.*+?^=!:${}()|\[\]\/\\])/g, '\\$1'), 'gi');
  for (const item of jumpListItems) {
    var name = item.name;
    if (filter) {
      // Boldify the substring of name matching the filter.
      name = name.replace(re, function (s) { return '<b>' + s + '</b>'; });
      if (name == item.name) {
        // We didn't change name, so it didn't match the filter.
        continue;
      }
    }
    // The text we display includes the name (with filter match bolded), and the
    // kind in italics.
    item.link.innerHTML = name + ' <i>' + item.kind + '</i>';
    jumpList.appendChild(item.link);
  }
  jumpBody.scrollTop = 0;
  if (jumpList.children.length > 0) {
    setActiveJumpItem(0);
  }
}

// Set the active jump item to n.
function setActiveJumpItem(n) {
  const cs = jumpList.children;
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
function incActiveJumpItem(delta) {
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
jumpFilter.addEventListener('keyup', function(event) {
  if (jumpFilter.value.toUpperCase() != lastFilterValue.toUpperCase()) {
    updateJumpList(jumpFilter.value);
  }
});

// Pressing enter in the filter selects the first element in the list.
// TODO(b/143454398) add arrow keys and track the active list element.
jumpFilter.addEventListener('keydown', function(event) {
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
        jumpList.children[activeJumpItem].click();
      }
      break;
  }
});

const shortcutsDialog = document.querySelector('.ShortcutsDialog');
if (!shortcutsDialog.showModal) {
  dialogPolyfill.registerDialog(shortcutsDialog);
}


// Keyboard shortcuts:
// - Pressing 'f' opens the jump-to-identifier dialog.
// - Pressing '?' opens up the shortcut dialog.
// Ignore a keypress if a dialog is already open, or if it is pressed on a
// component that wants to consume it.
document.addEventListener('keypress', function (e) {
  if (jumpDialog.open || shortcutsDialog.open) return;
  const t = e.target.tagName;
  if (t == 'INPUT' || t == 'SELECT' || t == 'TEXTAREA' ) return;
  if (e.target.contentEditable && e.target.contentEditable == 'true') return;
  if (e.metaKey || e.ctrlKey) return;
  const ch = String.fromCharCode(e.which);
  switch (ch) {
    case 'f':
      e.preventDefault();
      jumpFilter.value = '';
      jumpDialog.showModal();
      updateJumpList('');
      break;
    case '?':
      shortcutsDialog.showModal();
      break;
  }
});
