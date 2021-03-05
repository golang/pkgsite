'use strict';
/*!
 * @license
 * Copyright 2019-2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
const jumpDialog = document.querySelector('.JumpDialog');
const jumpBody =
  jumpDialog === null || jumpDialog === void 0
    ? void 0
    : jumpDialog.querySelector('.JumpDialog-body');
const jumpList =
  jumpDialog === null || jumpDialog === void 0
    ? void 0
    : jumpDialog.querySelector('.JumpDialog-list');
const jumpFilter =
  jumpDialog === null || jumpDialog === void 0
    ? void 0
    : jumpDialog.querySelector('.JumpDialog-input');
const searchInput = document.querySelector('.js-searchFocus');
const doc = document.querySelector('.js-documentation');
if (jumpDialog && !jumpDialog.showModal) {
  dialogPolyfill.registerDialog(jumpDialog);
}
let jumpListItems;
function collectJumpListItems() {
  const items = [];
  if (!doc) return;
  for (const el of doc.querySelectorAll('[data-kind]')) {
    items.push(newJumpListItem(el));
  }
  for (const item of items) {
    item.link.addEventListener('click', function () {
      jumpDialog === null || jumpDialog === void 0 ? void 0 : jumpDialog.close();
    });
  }
  items.sort(function (a, b) {
    return a.lower.localeCompare(b.lower);
  });
  return items;
}
function newJumpListItem(el) {
  var _a;
  const a = document.createElement('a');
  const name = el.getAttribute('id');
  a.setAttribute('href', '#' + name);
  a.setAttribute('tabindex', '-1');
  const kind = el.getAttribute('data-kind');
  return {
    link: a,
    name: name !== null && name !== void 0 ? name : '',
    kind: kind !== null && kind !== void 0 ? kind : '',
    lower:
      (_a = name === null || name === void 0 ? void 0 : name.toLowerCase()) !== null &&
      _a !== void 0
        ? _a
        : '',
  };
}
let lastFilterValue;
let activeJumpItem = -1;
function updateJumpList(filter) {
  lastFilterValue = filter;
  if (!jumpListItems) {
    jumpListItems = collectJumpListItems();
  }
  setActiveJumpItem(-1);
  while (jumpList === null || jumpList === void 0 ? void 0 : jumpList.firstChild) {
    jumpList.firstChild.remove();
  }
  if (filter) {
    const filterLowerCase = filter.toLowerCase();
    const exactMatches = [];
    const prefixMatches = [];
    const infixMatches = [];
    const makeLinkHtml = (item, boldStart, boldEnd) => {
      return (
        item.name.substring(0, boldStart) +
        '<b>' +
        item.name.substring(boldStart, boldEnd) +
        '</b>' +
        item.name.substring(boldEnd)
      );
    };
    for (const item of jumpListItems !== null && jumpListItems !== void 0 ? jumpListItems : []) {
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
      jumpList === null || jumpList === void 0 ? void 0 : jumpList.appendChild(item.link);
    }
  } else {
    for (const item of jumpListItems !== null && jumpListItems !== void 0 ? jumpListItems : []) {
      item.link.innerHTML = item.name + ' <i>' + item.kind + '</i>';
      jumpList === null || jumpList === void 0 ? void 0 : jumpList.appendChild(item.link);
    }
  }
  if (jumpBody) {
    jumpBody.scrollTop = 0;
  }
  if (jumpList && jumpList.children.length > 0) {
    setActiveJumpItem(0);
  }
}
function setActiveJumpItem(n) {
  const cs = jumpList === null || jumpList === void 0 ? void 0 : jumpList.children;
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
    const activeTop = cs[n].offsetTop - cs[0].offsetTop;
    const activeBottom = activeTop + cs[n].clientHeight;
    if (activeTop < jumpBody.scrollTop) {
      jumpBody.scrollTop = activeTop;
    } else if (activeBottom > jumpBody.scrollTop + jumpBody.clientHeight) {
      jumpBody.scrollTop = activeBottom - jumpBody.clientHeight;
    }
  }
  activeJumpItem = n;
}
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
jumpFilter === null || jumpFilter === void 0
  ? void 0
  : jumpFilter.addEventListener('keyup', function () {
      if (jumpFilter.value.toUpperCase() != lastFilterValue.toUpperCase()) {
        updateJumpList(jumpFilter.value);
      }
    });
jumpFilter === null || jumpFilter === void 0
  ? void 0
  : jumpFilter.addEventListener('keydown', function (event) {
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
              jumpList.children[activeJumpItem].click();
            }
          }
          break;
      }
    });
const shortcutsDialog = document.querySelector('.ShortcutsDialog');
if (shortcutsDialog && !shortcutsDialog.showModal) {
  dialogPolyfill.registerDialog(shortcutsDialog);
}
document.addEventListener('keypress', function (e) {
  if (
    (jumpDialog === null || jumpDialog === void 0 ? void 0 : jumpDialog.open) ||
    (shortcutsDialog === null || shortcutsDialog === void 0 ? void 0 : shortcutsDialog.open) ||
    !doc
  ) {
    return;
  }
  const target = e.target;
  const t = target === null || target === void 0 ? void 0 : target.tagName;
  if (t == 'INPUT' || t == 'SELECT' || t == 'TEXTAREA') {
    return;
  }
  if ((target === null || target === void 0 ? void 0 : target.contentEditable) == 'true') {
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
      jumpDialog === null || jumpDialog === void 0 ? void 0 : jumpDialog.showModal();
      updateJumpList('');
      break;
    case '?':
      shortcutsDialog === null || shortcutsDialog === void 0 ? void 0 : shortcutsDialog.showModal();
      break;
    case '/':
      if (searchInput && !window.navigator.userAgent.includes('Firefox')) {
        e.preventDefault();
        searchInput.focus();
      }
      break;
  }
});
const jumpOutlineInput = document.querySelector('.js-jumpToInput');
if (jumpOutlineInput) {
  jumpOutlineInput.addEventListener('click', () => {
    if (jumpFilter) {
      jumpFilter.value = '';
    }
    jumpDialog === null || jumpDialog === void 0 ? void 0 : jumpDialog.showModal();
    updateJumpList('');
  });
}
//# sourceMappingURL=jump.js.map
