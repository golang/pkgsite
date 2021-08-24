/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */class m{constructor(n){this.el=n;this.el.addEventListener("change",t=>{const o=t.target;let a=o.value;o.value.startsWith("/")||(a="/"+a),window.location.href=a})}}function d(c){const n=document.createElement("label");n.classList.add("go-Label"),n.setAttribute("aria-label","Menu");const t=document.createElement("select");t.classList.add("go-Select","js-selectNav"),n.appendChild(t);const o=document.createElement("optgroup");o.label="Outline",t.appendChild(o);const a={};let l;for(const e of c.treeitems){if(Number(e.depth)>4)continue;e.groupTreeitem?(l=a[e.groupTreeitem.label],l||(l=a[e.groupTreeitem.label]=document.createElement("optgroup"),l.label=e.groupTreeitem.label,t.appendChild(l))):l=o;const r=document.createElement("option");r.label=e.label,r.textContent=e.label,r.value=e.el.href.replace(window.location.origin,"").replace("/",""),l.appendChild(r)}return c.addObserver(e=>{var i;const r=e.el.hash,s=(i=t.querySelector(`[value$="${r}"]`))==null?void 0:i.value;s&&(t.value=s)},50),n}export{m as SelectNavController,d as makeSelectNav};
//# sourceMappingURL=select.js.map
