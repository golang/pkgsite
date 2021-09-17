/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { CarouselController } from '../../shared/carousel/carousel';
import { SelectNavController, makeSelectNav } from '../../shared/outline/select';
import { TreeNavController } from '../../shared/outline/tree';

window.addEventListener('load', () => {
  const tree = document.querySelector<HTMLElement>('.js-tree');
  if (tree) {
    const treeCtrl = new TreeNavController(tree);
    const select = makeSelectNav(treeCtrl);
    document.querySelector('.js-mainNavMobile')?.appendChild(select);
  }

  const guideTree = document.querySelector<HTMLElement>('.Outline .js-tree');
  if (guideTree) {
    const treeCtrl = new TreeNavController(guideTree);
    const select = makeSelectNav(treeCtrl);
    document.querySelector('.Outline .js-select')?.appendChild(select);
  }

  for (const el of document.querySelectorAll('.js-toggleTheme')) {
    el.addEventListener('click', e => {
      const value = (e.currentTarget as HTMLButtonElement).getAttribute('data-value');
      document.documentElement.setAttribute('data-theme', String(value));
    });
  }
  for (const el of document.querySelectorAll('.js-toggleLayout')) {
    el.addEventListener('click', e => {
      const value = (e.currentTarget as HTMLButtonElement).getAttribute('data-value');
      document.documentElement.setAttribute('data-layout', String(value));
    });
  }

  for (const el of document.querySelectorAll<HTMLSelectElement>('.js-selectNav')) {
    new SelectNavController(el);
  }

  for (const el of document.querySelectorAll<HTMLSelectElement>('.js-carousel')) {
    new CarouselController(el);
  }
});

customElements.define(
  'go-color',
  class extends HTMLElement {
    constructor() {
      super();
      this.style.setProperty('display', 'contents');
      // The current version of TypeScript is not aware of String.replaceAll.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const name = this.id as any;
      this.removeAttribute('id');
      this.innerHTML = `
        <div style="--color: var(${name});" class="GoColor-circle"></div>
        <span>
          <div id="${name}" class="go-textLabel GoColor-title">${name
        .replace('--color-', '')
        .replaceAll('-', ' ')}</div>
          <pre class="StringifyElement-markup">var(${name})</pre>
        </span>
      `;
      this.querySelector('pre')?.addEventListener('click', () => {
        navigator.clipboard.writeText(`var(${name})`);
      });
    }
  }
);

customElements.define(
  'go-icon',
  class extends HTMLElement {
    constructor() {
      super();
      this.style.setProperty('display', 'contents');
      // The current version of TypeScript is not aware of String.replaceAll.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const name = this.getAttribute('name') as any;
      this.innerHTML = `<p id="icon-${name}" class="go-textLabel GoIcon-title">${name.replaceAll(
        '_',
        ' '
      )}</p>
        <stringify-el>
          <img class="go-Icon" height="24" width="24" src="/static/shared/icon/${name}_gm_grey_24dp.svg" alt="">
        </stringify-el>
      `;
    }
  }
);

customElements.define(
  'clone-el',
  class extends HTMLElement {
    constructor() {
      super();
      this.style.setProperty('display', 'contents');
      const selector = this.getAttribute('selector');
      if (!selector) return;
      const html = '    ' + document.querySelector(selector)?.outerHTML;
      this.innerHTML = `
        <stringify-el collapsed>${html}</stringify-el>
      `;
    }
  }
);

customElements.define(
  'stringify-el',
  class extends HTMLElement {
    constructor() {
      super();
      this.style.setProperty('display', 'contents');
      const html = this.innerHTML;
      const idAttr = this.id ? ` id="${this.id}"` : '';
      this.removeAttribute('id');
      let markup = `<pre class="StringifyElement-markup">` + escape(trim(html)) + `</pre>`;
      if (this.hasAttribute('collapsed')) {
        markup = `<details class="StringifyElement-details"><summary>Markup</summary>${markup}</details>`;
      }
      this.innerHTML = `<span${idAttr}>${html}</span>${markup}`;
      this.querySelector('pre')?.addEventListener('click', () => {
        navigator.clipboard.writeText(html);
      });
    }
  }
);

/**
 * trim removes excess indentation from html markup by
 * measuring the number of spaces in the first line of
 * the given string and removing that number of spaces
 * from the beginning of each line.
 */
function trim(html: string) {
  return html
    .split('\n')
    .reduce<{ result: string[]; start: number }>(
      (acc, val) => {
        if (acc.result.length === 0) {
          const start = val.indexOf('<');
          acc.start = start === -1 ? 0 : start;
        }
        val = val.slice(acc.start);
        if (val) {
          acc.result.push(val);
        }
        return acc;
      },
      { result: [], start: 0 }
    )
    .result.join('\n');
}

function escape(html: string) {
  // The current version of TypeScript is not aware of String.replaceAll.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return (html as any)?.replaceAll('<', '&lt;')?.replaceAll('>', '&gt;');
}
