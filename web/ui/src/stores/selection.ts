import { reactive } from 'vue'

// Shared selection: which resource the detail drawer shows. Plain reactive
// singleton, same pattern as the other stores.

interface Selected {
  kind: string
  name: string
}

interface SelectionState {
  current: Selected | null
}

const state = reactive<SelectionState>({ current: null })

function select(kind: string, name: string): void {
  state.current = { kind, name }
}

function clear(): void {
  state.current = null
}

export function useSelection() {
  return { state, select, clear }
}
