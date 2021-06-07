/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { TreeNavController } from './tree.js';

export class SelectNavController {
  constructor(private el: Element) {
    this.el.addEventListener('change', e => {
      const target = e.target as HTMLSelectElement;
      let href = target.value;
      if (!target.value.startsWith('/')) {
        href = '/' + href;
      }
      window.location.href = href;
    });
  }
}

export function makeSelectNav(tree: TreeNavController): HTMLLabelElement {
  const label = document.createElement('label');
  label.classList.add('go-Label');
  label.setAttribute('aria-label', 'Menu');
  const select = document.createElement('select');
  select.classList.add('go-Select', 'js-selectNav');
  label.appendChild(select);
  const outline = document.createElement('optgroup');
  outline.label = 'Outline';
  select.appendChild(outline);
  const groupMap = {};
  let group: HTMLOptGroupElement;
  for (const t of tree.treeitems) {
    if (Number(t.depth) > 4) continue;
    if (t.groupTreeitem) {
      group = groupMap[t.groupTreeitem.label];
      if (!group) {
        group = groupMap[t.groupTreeitem.label] = document.createElement('optgroup');
        group.label = t.groupTreeitem.label;
        select.appendChild(group);
      }
    } else {
      group = outline;
    }
    const o = document.createElement('option');
    o.label = t.label;
    o.textContent = t.label;
    o.value = (t.el as HTMLAnchorElement).href.replace(window.location.origin, '').replace('/', '');
    group.appendChild(o);
  }
  tree.addObserver(t => {
    const value = select.querySelector<HTMLOptionElement>(`[label="${t.label}"]`)?.value;
    if (value) {
      select.value = value;
    }
  }, 50);
  return label;
}
