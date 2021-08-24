/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */class s{constructor(l){this.el=l;!window.HTMLDialogElement&&!l.showModal&&import("../../../third_party/dialog-polyfill/dialog-polyfill.esm.js").then(({default:e})=>{e.registerDialog(l)});const t=l.id,o=document.querySelector(`[aria-controls="${t}"]`);o&&o.addEventListener("click",()=>{var e;this.el.showModal?this.el.showModal():this.el.open=!0,(e=l.querySelector("input"))==null||e.focus()});for(const e of this.el.querySelectorAll("[data-modal-close]"))e.addEventListener("click",()=>{this.el.close?this.el.close():this.el.open=!1})}}export{s as ModalController};
//# sourceMappingURL=modal.js.map
