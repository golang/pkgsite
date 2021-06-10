/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */export class SelectNavController{constructor(n){this.el=n;this.el.addEventListener("change",t=>{const r=t.target;let a=r.value;r.value.startsWith("/")||(a="/"+a),window.location.href=a})}}export function makeSelectNav(c){const n=document.createElement("label");n.classList.add("go-Label"),n.setAttribute("aria-label","Menu");const t=document.createElement("select");t.classList.add("go-Select","js-selectNav"),n.appendChild(t);const r=document.createElement("optgroup");r.label="Outline",t.appendChild(r);const a={};let l;for(const e of c.treeitems){if(Number(e.depth)>4)continue;e.groupTreeitem?(l=a[e.groupTreeitem.label],l||(l=a[e.groupTreeitem.label]=document.createElement("optgroup"),l.label=e.groupTreeitem.label,t.appendChild(l))):l=r;const o=document.createElement("option");o.label=e.label,o.textContent=e.label,o.value=e.el.href.replace(window.location.origin,"").replace("/",""),l.appendChild(o)}return c.addObserver(e=>{const o=t.querySelector(`[label="${e.label}"]`)?.value;o&&(t.value=o)},50),n}
//# sourceMappingURL=select.js.map
