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
      window.location.href = target.value;
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
  const o = document.createElement('option');
  o.disabled = true;
  o.selected = true;
  o.label = 'Outline';
  select.appendChild(o);
  let group: HTMLOptGroupElement;
  for (const [i, t] of tree.treeitems.entries()) {
    if (Number(t.depth) > 2) continue;
    if (t.depth === 1 && tree.treeitems[i + 1]?.depth > 1) {
      group = document.createElement('optgroup');
      group.label = t.label;
      select.appendChild(group);
    } else {
      const o = document.createElement('option');
      o.label = t.label;
      o.textContent = t.label;
      o.value = (t.el as HTMLAnchorElement).href
        .replace(window.location.origin, '')
        .replace('/', '');
      if (t.depth === 1) {
        select.appendChild(o);
      } else {
        group.appendChild(o);
      }
    }
  }
  tree.addObserver(t => {
    const value = select.querySelector<HTMLOptionElement>(`[label="${t.label}"]`)?.value;
    if (value) {
      select.value = value;
    }
  }, 50);
  return label;
}
