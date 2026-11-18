// One row of a `static_data` artifact served by GET /api/v1/static-data/{id} — see
// internal/staticdata (nsw-srilanka backend). Every row has const (the value a field stores)
// and title (its label). A row may carry other keys, which a displayTemplate can use and a
// field's x-search.params.searchKeys can name for a typed search.
export interface StaticDataRow {
  const: string
  title: string
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
