// One row of a `static_data` artifact served by GET /api/v1/static-data/{id} — see
// internal/staticdata (nsw-srilanka backend). A row is a flat object with any keys: a form
// field picks the key it stores (x-search.params.valueKey) and the keys a typed search
// matches (x-search.params.searchKeys), and its displayTemplate can use any key.
export interface StaticDataRow {
  // Values of the sibling field this row belongs to (e.g. commodity codes).
  // The renderer reads x-search.dependsOn and sends that field's current value
  // as params.parent; search() keeps rows whose parents include it.
  parents?: string[]
  [key: string]: unknown
}

// Wire envelope for a static_data artifact response: always an object with a
// top-level "data" array, never a bare array — see internal/staticdata.Parse.
export interface StaticDataResponse {
  data: StaticDataRow[]
}
