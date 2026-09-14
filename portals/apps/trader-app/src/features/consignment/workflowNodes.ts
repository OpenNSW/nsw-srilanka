// Node types the trader never acts on. ActionListView and Next Task share this
// so a new non-trader type only needs to be added here.
const HIDDEN_NODE_TYPES = new Set(['START', 'END', 'GATEWAY', 'END_NODE', 'SYSTEM', 'SPLIT_TASK'])

export function isTraderVisibleNodeType(type: string | undefined): boolean {
  return !HIDDEN_NODE_TYPES.has((type ?? '').toUpperCase())
}
