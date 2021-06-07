/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */export class SelectNavController{constructor(n){this.el=n;this.el.addEventListener("change",t=>{const o=t.target;let r=o.value;o.value.startsWith("/")||(r="/"+r),window.location.href=r})}}export function makeSelectNav(c){const n=document.createElement("label");n.classList.add("go-Label"),n.setAttribute("aria-label","Menu");const t=document.createElement("select");t.classList.add("go-Select","js-selectNav"),n.appendChild(t);const o=document.createElement("optgroup");o.label="Outline",t.appendChild(o);const r={};let l;for(const e of c.treeitems){if(Number(e.depth)>4)continue;e.groupTreeitem?(l=r[e.groupTreeitem.label],l||(l=r[e.groupTreeitem.label]=document.createElement("optgroup"),l.label=e.groupTreeitem.label,t.appendChild(l))):l=o;const a=document.createElement("option");a.label=e.label,a.textContent=e.label,a.value=e.el.href.replace(window.location.origin,"").replace("/",""),l.appendChild(a)}return c.addObserver(e=>{const a=t.querySelector(`[label="${e.label}"]`)?.value;a&&(t.value=a)},50),n}
//# sourceMappingURL=select.js.map
