/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */export class ModalController{constructor(e){this.el=e;!window.HTMLDialogElement&&!e.showModal&&import("../../../third_party/dialog-polyfill/dialog-polyfill.esm.js").then(({default:l})=>{l.registerDialog(e)});const t=e.id,o=document.querySelector(`[aria-controls="${t}"]`);o&&o.addEventListener("click",()=>{this.el.showModal?this.el.showModal():this.el.open=!0,e.querySelector("input")?.focus()});for(const l of this.el.querySelectorAll("[data-modal-close]"))l.addEventListener("click",()=>{this.el.close?this.el.close():this.el.open=!1})}}
//# sourceMappingURL=modal.js.map
