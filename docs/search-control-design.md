# SearchSelectControl — Design Comparison

Two approaches for how the search/typeahead control sources its data.

- **Approach A — Declarative `x-search` config (current PR #599):** the control owns
  fetching, pagination, and response parsing, driven by config embedded in the JSON schema.
- **Approach B — Service hook / registry:** the control owns only the UI; a registered
  *service* owns fetching, pagination, and everything transport-related.

The goal of this doc is to help the team pick a direction (or the hybrid).

---

## TL;DR

|                               | A — Declarative `x-search`                         | B — Service hook                                       |
|-------------------------------|----------------------------------------------------|--------------------------------------------------------|
| Who fetches data              | The control                                        | A consumer-registered service                          |
| Who owns pagination           | The control (offset/cursor/none modes)             | The service (opaque token)                             |
| Add a new search field        | **Backend only** — emit schema, no frontend change | **Frontend deploy** — register a service               |
| Pagination styles supported   | offset (item-based), cursor-in-body, none          | **Anything** (page#, Link header, GraphQL, in-memory…) |
| Saved-value → label hydration | Not supported (shows raw value)                    | Supported via `resolve()`                              |
| Control complexity            | High (~450 lines: fetch + state machine)           | Low (UI + debounce + abort only)                       |
| Type safety of data layer     | Stringly-typed paths in JSON                       | Typed service per consumer                             |
| Coupling                      | Schema is self-contained                           | Schema references a name the frontend must provide     |

**Recommendation:** invert to B for the data layer, but keep a built-in HTTP service
factory so the simple, backend-defined case from A still works with zero frontend code.
See [Hybrid](#approach-c--hybrid-recommended).

---

## Approach A — Declarative `x-search` config (current)

The schema carries everything the control needs to fetch and paginate. The frontend
only provides a `baseUrl` + auth headers via `SearchContext`.

### JSONForms configuration

```jsonc
// JSON Schema
{
  "type": "object",
  "properties": {
    "country": {
      "type": "string",
      "description": "Type to search",
      "x-search": {
        "path": "/api/countries-offset",
        "valueKey": "id",
        "labelKey": "name",
        "pagination": "offset",   // "none" | "offset" | "cursor"
        "pageSize": 5,
        "itemsPath": "items",     // where the array lives in the response
        "searchParam": "q",
        "limitParam": "limit",
        "offsetParam": "offset",
        "cursorParam": "cursor",
        "nextCursorPath": "nextCursor",
        "loadOnOpen": false
      }
    }
  },
  "required": ["country"]
}
```

```jsonc
// UI Schema
{
  "type": "VerticalLayout",
  "elements": [
    { "type": "Control", "scope": "#/properties/country",
      "options": { "placeholder": "Search for a country…" } }
  ]
}
```

```tsx
// App wiring — one provider for the whole form
<SearchProvider baseUrl="https://api.example.com" getHeaders={getAuthHeaders}>
  <JsonForms schema={schema} uischema={uischema} renderers={radixRenderers} … />
</SearchProvider>
```

### Pros
- **Backend-defined fields.** A new searchable field ships as schema only — no frontend
  code, no redeploy. Strong fit for dynamically-defined forms (OGA-style).
- Self-contained schema; the contract travels with the data.
- Zero per-field wiring in the app beyond one `SearchProvider`.

### Cons
- The control carries fetch + a pagination state machine (~450 lines) → harder to test/maintain.
- Only the pagination shapes we coded for. **Page-number pagination, Link-header cursors,
  total-count-driven "has more", POST/body search are all unrepresentable.**
- Stringly-typed (`valueKey`, `itemsPath`, `nextCursorPath`) — typos fail at runtime.
- No way to hydrate a saved value's label (a restored `id` shows as the raw id).

---

## Approach B — Service hook / registry

The schema only names a service. Consumers register services; each service fully owns
data fetching and pagination. The control treats the continuation token as **opaque** —
it never knows whether it's an offset, a page number, or a cursor.

### Service contract

```ts
interface SearchOption { value: string; label: string }

interface SearchService {
  search(args: {
    query: string
    cursor?: unknown            // opaque — control hands back whatever the service last returned
    signal: AbortSignal
  }): Promise<{ options: SearchOption[]; nextCursor?: unknown }>

  // optional: turn a saved value back into a labelled option
  resolve?(value: string): Promise<SearchOption | undefined>
}
```

`hasMore` is simply `nextCursor !== undefined`. The control's job shrinks to: input,
debounce, abort, dropdown rendering, selection, and "call `search` again with the last
`nextCursor`."

### JSONForms configuration

```jsonc
// JSON Schema — the data layer is gone; only a service name + UI hints remain
{
  "type": "object",
  "properties": {
    "country": {
      "type": "string",
      "description": "Type to search",
      "x-search": { "service": "countries", "pageSize": 5 }
    }
  },
  "required": ["country"]
}
```

```jsonc
// UI Schema — unchanged
{
  "type": "VerticalLayout",
  "elements": [
    { "type": "Control", "scope": "#/properties/country",
      "options": { "placeholder": "Search for a country…" } }
  ]
}
```

```tsx
// App wiring — register the services the schema can reference
const services: Record<string, SearchService> = {
  countries: {
    async search({ query, cursor, signal }) {
      const page = (cursor as number) ?? 1            // service owns pagination — page-number here
      const res = await api.get('/countries', { params: { q: query, page }, signal })
      return {
        options: res.data.results.map(c => ({ value: c.id, label: `${c.name} (${c.code})` })),
        nextCursor: res.data.hasNextPage ? page + 1 : undefined,
      }
    },
    async resolve(id) {
      const c = await api.get(`/countries/${id}`)
      return { value: c.data.id, label: c.data.name }
    },
  },
}

<SearchServiceProvider services={services}>
  <JsonForms schema={schema} uischema={uischema} renderers={radixRenderers} … />
</SearchServiceProvider>
```

Note how the example uses **page-number** pagination and a **composite label** — neither
is expressible in Approach A, but here it's just service code.

### Pros
- Control becomes small and single-responsibility → easy to test.
- **Any** transport/pagination: offset, page#, cursor, Link header, GraphQL, in-memory filter.
- Typed contract per consumer; no stringly-typed paths.
- `resolve()` fixes saved-value → label hydration.
- Auth/clients are closed over by the service; no special `baseUrl`/`getHeaders` plumbing.

### Cons
- **Loses backend-only fields.** Every searchable field needs a service registered in the
  frontend bundle → a new field means a frontend deploy. Significant if forms are
  backend-defined.
- Schema references a name the frontend must supply → missing service is a runtime error;
  needs a sensible fallback/error state.
- A bit more boilerplate for the trivial case.

---

## Approach C — Hybrid (recommended)

Make the **service contract the only thing the control talks to**, *and* ship a built-in
HTTP service factory in the package that reproduces Approach A's behavior. The control
internally converts an `x-search` with a `path` into a default service via the factory.

- Simple, backend-defined case → still works with **zero consumer code** (factory kicks in).
- Hard cases → register a custom service.
- The control stays small and pagination-agnostic either way.

### How config disambiguates

```jsonc
// Backend-defined, uses the built-in HTTP factory (≈ Approach A)
"x-search": { "path": "/api/countries-offset", "valueKey": "id", "labelKey": "name", "pagination": "offset" }

// Custom service registered in the frontend (≈ Approach B)
"x-search": { "service": "countries" }
```

The control: if `service` is present → look it up in the registry; else if `path` is
present → build a default HTTP service from the factory. Both paths produce a
`SearchService`; the control only ever calls `search()`/`resolve()`.

---

## The key decision for the team

> **Do we need search fields that are defined entirely by the backend, with no frontend
> registration?**

- **Yes** → Hybrid (C): service contract + keep the HTTP factory bridge.
- **No** → Full B: drop the `path`/fetch config from the schema; the control gets much smaller.

Either way, the control should depend on the `SearchService` interface — not on `fetch`
and a hand-rolled pagination state machine.