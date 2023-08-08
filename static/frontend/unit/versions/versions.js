var i=class{constructor(){this.expand=document.querySelector(".js-versionsExpand");this.collapse=document.querySelector(".js-versionsCollapse");this.details=[...document.querySelectorAll(".js-versionDetails")];var n,e,s;if((n=this.expand)!=null&&n.parentElement){this.details.some(t=>t.tagName==="DETAILS")&&(this.expand.parentElement.style.display="block");for(let t of this.details)t.addEventListener("click",()=>{this.updateButtons()});(e=this.expand)==null||e.addEventListener("click",()=>{this.details.map(t=>t.open=!0),this.updateButtons()}),(s=this.collapse)==null||s.addEventListener("click",()=>{this.details.map(t=>t.open=!1),this.updateButtons()}),this.updateButtons(),this.setCurrent()}}setCurrent(){var s,t;let n=(t=(s=document.querySelector(".js-canonicalURLPath"))==null?void 0:s.dataset)==null?void 0:t.canonicalUrlPath,e=document.querySelector(`.js-versionLink[href="${n}"]`);e&&(e.style.fontWeight="bold")}updateButtons(){setTimeout(()=>{if(!this.expand||!this.collapse)return;let n,e;for(let s of this.details)n=n||s.open,e=e||!s.open;this.expand.style.display=e?"inline-block":"none",this.collapse.style.display=e?"none":"inline-block"})}};new i;export{i as VersionsController};
/*!
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=versions.js.map
