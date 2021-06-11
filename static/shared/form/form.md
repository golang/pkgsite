## Forms

---

### Input {#form-input}

```html
<input class="go-Input" placeholder="Placeholder text" />
```

### Input w/ Label {#form-input-label}

```html
<label class="go-Label"
  >URL
  <input class="go-Input" placeholder="e.g., https://pkg.go.dev/net/http" />
</label>
```

### Select w/ Inline Label {#form-select}

```html
<label class="go-Label go-Label--inline">
  Rendered for
  <select class="go-Select">
    <option>linux/amd64</option>
    <option>windows/amd64</option>
    <option>darwin/amd64</option>
    <option>js/wasm</option>
  </select>
</label>
```

### Input Groups {#form-groups}

```html
<form
  class="go-InputGroup"
  action="#forms"
  data-gtmc="search form"
  aria-label="Search for a package"
  role="search"
>
  <input name="q" class="go-Input" placeholder="Search for a package" />
  <button class="go-Button go-Button--inverted" aria-label="Submit search">
    <img
      class="go-Icon"
      height="24"
      width="24"
      src="/static/_icon/search_gm_grey_24dp.svg"
      alt=""
    />
  </button>
</form>
```

```html
<form
  class="go-InputGroup"
  action="#forms"
  data-gtmc="search form"
  aria-label="Search for a package"
  role="search"
>
  <input name="q" class="go-Input" placeholder="Search for a package" />
  <button class="go-Button">Submit</button>
</form>
```

```html
<div class="go-InputGroup">
  <button class="go-Button go-Button--inverted" data-gtmc="menu button" aria-label="Open menu">
    <img
      class="go-Icon"
      height="24"
      width="24"
      src="/static/_icon/filter_list_gm_grey_24dp.svg"
      alt=""
    />
  </button>
  <button class="go-Button go-Button--inverted" data-gtmc="menu button">Share</button>
  <button class="go-Button go-Button--inverted" data-gtmc="menu button">Format</button>
  <button class="go-Button go-Button--inverted" data-gtmc="menu button">Run</button>
  <button class="go-Button go-Button--inverted" data-gtmc="menu button" aria-label="Search">
    <img
      class="go-Icon"
      height="24"
      width="24"
      src="/static/_icon/search_gm_grey_24dp.svg"
      alt=""
    />
  </button>
</div>
```

```html
<fieldset class="go-Label go-Label--inline">
  <legend>Label</legend>
  <div class="go-InputGroup">
    <button class="go-Button go-Button--inverted" data-gtmc="menu button" aria-label="Open menu">
      <img
        class="go-Icon"
        height="24"
        width="24"
        src="/static/_icon/filter_list_gm_grey_24dp.svg"
        alt=""
      />
    </button>
    <button class="go-Button go-Button--inverted" data-gtmc="menu button" aria-label="Search">
      <img
        class="go-Icon"
        height="24"
        width="24"
        src="/static/_icon/search_gm_grey_24dp.svg"
        alt=""
      />
    </button>
  </div>
</fieldset>
```
