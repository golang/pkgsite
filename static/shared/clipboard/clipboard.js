/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */export class ClipboardController{constructor(t){this.el=t;var e,o,i,a,s;this.data=(e=t.dataset.toCopy)!=null?e:t.innerText,!this.data&&((o=t.parentElement)==null?void 0:o.classList.contains("go-InputGroup"))&&(this.data=(s=this.data||((a=(i=t.parentElement)==null?void 0:i.querySelector("input"))==null?void 0:a.value))!=null?s:""),t.addEventListener("click",n=>this.handleCopyClick(n))}handleCopyClick(t){t.preventDefault();const e=1e3;if(!navigator.clipboard){this.showTooltipText("Unable to copy",e);return}navigator.clipboard.writeText(this.data).then(()=>{this.showTooltipText("Copied!",e)}).catch(()=>{this.showTooltipText("Unable to copy",e)})}showTooltipText(t,e){this.el.setAttribute("data-tooltip",t),setTimeout(()=>this.el.setAttribute("data-tooltip",""),e)}}
//# sourceMappingURL=clipboard.js.map
