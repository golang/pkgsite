function n(o,s){let e=document.querySelector(`form[name="${o}" ]`);if(!e)throw Error(`Form "${o}" not found.`);e.result.value="request pending...";let t=new XMLHttpRequest;t.onreadystatechange=function(){this.readyState==4&&(this.status>=200&&this.status<300?s?location.reload():e.result.value="Success.":e.result.value="ERROR: "+this.responseText)},t.open(e.method,e.action),t.send(new FormData(e))}window.submitForm=n;
/*!
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=worker.js.map
